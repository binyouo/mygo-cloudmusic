package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A confirmation received after the QR dialog was closed or replaced cannot persist its session.
func TestLoginDiscardLateConfirmation(t *testing.T) {
	a := newTestApp()
	result := netease.QRLoginStatus{Code: netease.QRConfirmed, Profile: &netease.Profile{UserID: 42}}
	commit := func() error { t.Fatal("stale confirmation tried to persist its session"); return nil }
	a.login = loginState{gen: 1}
	if !a.applyLoginCheck(1, result, nil, commit) {
		t.Fatal("polling continued after closing the dialog")
	}
	a.login = loginState{open: true, gen: 2, status: netease.QRWaiting}
	if !a.applyLoginCheck(1, result, nil, commit) || a.login.status != netease.QRWaiting {
		t.Fatal("confirmation of an obsolete code changed the replacement")
	}
}

// Startup restoration can neither replace a newer scan's account nor clear its session when the saved login was expired.
func TestLoginIgnoresObsoleteAccountRestore(t *testing.T) {
	for _, saved := range []*netease.SavedAccount{{Profile: &netease.Profile{UserID: 1}}, {}} {
		a := newTestApp()
		a.accountRevision = 2
		a.profile = &netease.Profile{UserID: 42}
		a.applyRestoredAccount(1, saved, nil)
		if a.profile == nil || a.profile.UserID != 42 || a.accountRevision != 2 {
			t.Fatal("stale startup restoration changed the active account")
		}
	}
}

// A home request from the previous session cannot replace recommendations or finish the current account's load.
func TestLoginShowsOnlyCurrentAccountRecommendations(t *testing.T) {
	a := newTestApp()
	a.accountRevision, a.homeLoading = 2, true
	a.homeLoaded(1, homeFeed{
		recommended: []netease.Playlist{{ID: 1}}, daily: []netease.Track{{ID: 11}}, loaded: true,
	})
	if len(a.recPlaylists) != 0 || len(a.daily) != 0 || !a.homeLoading {
		t.Fatal("old recommendations reached the new account's home")
	}
	a.homeLoaded(2, homeFeed{
		recommended: []netease.Playlist{{ID: 2}}, daily: []netease.Track{{ID: 22}}, loaded: true,
	})
	if len(a.recPlaylists) != 1 || a.recPlaylists[0].ID != 2 || len(a.daily) != 1 || a.daily[0].ID != 22 || a.homeLoading {
		t.Fatal("the current account's recommendations did not reach the home page")
	}
}

// Closing the phone's login dialog cancels its current requests and discards any queued check on return to the app.
func TestPhoneCloseLoginCancelsPolling(t *testing.T) {
	a := newTestApp()
	a.phone = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.login = loginState{open: true, gen: 1, cancel: cancel, poll: &loginPoll{wake: make(chan struct{}, 1)}}
	tt := ui.NewTester(a.view, 667, 375)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.setLoginPollingActive(true)
	tapPhone(t, tt, "关闭登录")
	a.setLoginPollingActive(true)
	if a.login.open || ctx.Err() == nil || a.login.poll != nil {
		t.Fatal("closing the phone dialog left the login request or wakeup active")
	}
}

// A background timer tick does not authorize another check, and coming back wakes the same QR attempt without a tick.
func TestPhoneLoginPollSuspendsAndResumes(t *testing.T) {
	a := newTestApp()
	poll := &loginPoll{wake: make(chan struct{}, 1)}
	poll.active.Store(true)
	a.login = loginState{open: true, gen: 7, poll: poll, status: netease.QRScanned}
	a.setLoginPollingActive(false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ticks := make(chan time.Time)
	result := make(chan bool, 1)
	go func() { result <- poll.wait(ctx, ticks) }()
	select {
	case ticks <- time.Time{}:
	case allowed := <-result:
		t.Fatalf("background wait returned before a timer tick: %t", allowed)
	case <-time.After(time.Second):
		t.Fatal("poll worker did not receive its timer tick")
	}
	cancel()
	if allowed := wait(t, result, "the background poll to stop"); allowed {
		t.Fatal("background tick started a login check")
	}
	a.setLoginPollingActive(true)
	go func() { result <- poll.wait(t.Context(), nil) }()
	if allowed := wait(t, result, "the immediate foreground check"); !allowed || a.login.poll != poll || a.login.gen != 7 || a.login.status != netease.QRScanned {
		t.Fatal("returning to the app did not immediately continue the same scan")
	}
}

// The credentials form sends the trimmed account and the password to the check, and an accepted check commits its session.
func TestPasswordLoginCommitsAndSignsIn(t *testing.T) {
	a := newTestApp()
	oldCheck, oldPlays := loginByPassword, recentPlays
	defer func() { loginByPassword, recentPlays = oldCheck, oldPlays }()
	recentPlays = func(int) ([]netease.Play, error) { return nil, nil }
	called := make(chan [2]string, 1)
	loginByPassword = func(_ context.Context, account, password string) (*netease.Profile, func() error, error) {
		called <- [2]string{account, password}
		return &netease.Profile{UserID: 42}, nil, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1, account: " u@x.com ", password: "pw"}
	a.submitPasswordLogin()
	if got := wait(t, called, "the credentials check"); got != [2]string{"u@x.com", "pw"} {
		t.Fatalf("the check got %v, not the trimmed form's", got)
	}
	if !a.login.busy {
		t.Fatal("the in-flight check did not hold the form busy")
	}
	committed := false
	a.applyPasswordLogin(1, &netease.Profile{UserID: 42}, func() error { committed = true; return nil }, nil)
	if !committed || a.profile == nil || a.profile.UserID != 42 || !a.login.finished || a.login.password != "" || a.login.busy {
		t.Fatal("the accepted check did not commit its session and sign in")
	}
}

// A refused credentials check shows NetEase's reason and frees the submit, without a commit.
func TestPasswordLoginShowsRefusal(t *testing.T) {
	a := newTestApp()
	a.login = loginState{open: true, gen: 1, tab: 1, busy: true}
	a.applyPasswordLogin(1, nil, func() error { t.Fatal("commit ran on a refused check"); return nil }, errors.New("密码错误"))
	if !a.login.open || a.login.finished || a.login.busy || a.login.err != "密码错误" || a.profile != nil {
		t.Fatal("a refused check did not surface its reason")
	}
}

// A check landing after the dialog restarted or closed cannot commit its session.
func TestPasswordLoginDiscardsLateResult(t *testing.T) {
	a := newTestApp()
	a.login = loginState{open: true, gen: 2, tab: 1}
	commit := func() error { t.Fatal("a late result committed its session"); return nil }
	a.applyPasswordLogin(1, &netease.Profile{UserID: 42}, commit, nil)
	a.login.open = false
	a.applyPasswordLogin(2, &netease.Profile{UserID: 42}, commit, nil)
	if a.profile != nil || a.login.busy {
		t.Fatal("a late result touched the account")
	}
}

// The form refuses the network until both fields hold something.
func TestPasswordLoginRequiresBothFields(t *testing.T) {
	a := newTestApp()
	old := loginByPassword
	defer func() { loginByPassword = old }()
	loginByPassword = func(context.Context, string, string) (*netease.Profile, func() error, error) {
		t.Fatal("an incomplete form hit the network")
		return nil, nil, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1}
	a.submitPasswordLogin()
	if a.login.err == "" || a.login.busy {
		t.Fatal("the empty form went to the network")
	}
	a.login.account, a.login.err = "u@x.com", ""
	a.submitPasswordLogin()
	if a.login.err == "" || a.login.busy {
		t.Fatal("the passwordless form went to the network")
	}
}

// The SMS check signs a phone in with the code it was texted, committing its session like the password check.
func TestSMSLoginCommitsAndSignsIn(t *testing.T) {
	a := newTestApp()
	oldCheck, oldPlays := loginBySMS, recentPlays
	defer func() { loginBySMS, recentPlays = oldCheck, oldPlays }()
	recentPlays = func(int) ([]netease.Play, error) { return nil, nil }
	called := make(chan [2]string, 1)
	loginBySMS = func(_ context.Context, phone, code string) (*netease.Profile, func() error, error) {
		called <- [2]string{phone, code}
		return &netease.Profile{UserID: 42}, nil, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1, credMode: 1, account: "13800001111 ", smsCode: " 123456"}
	a.submitCredentials()
	if got := wait(t, called, "the code check"); got != [2]string{"13800001111", "123456"} {
		t.Fatalf("the check got %v, not the trimmed form's", got)
	}
	committed := false
	a.applyPasswordLogin(1, &netease.Profile{UserID: 42}, func() error { committed = true; return nil }, nil)
	if !committed || a.profile == nil || a.profile.UserID != 42 || !a.login.finished || a.login.smsCode != "" {
		t.Fatal("the accepted code check did not commit its session and sign in")
	}
}

// The code form refuses the network on an email address or an empty code.
func TestSMSLoginRequiresMobileAndCode(t *testing.T) {
	a := newTestApp()
	old := loginBySMS
	defer func() { loginBySMS = old }()
	loginBySMS = func(context.Context, string, string) (*netease.Profile, func() error, error) {
		t.Fatal("an incomplete code form hit the network")
		return nil, nil, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1, credMode: 1, account: "u@x.com", smsCode: "123456"}
	a.submitCredentials()
	if a.login.err == "" || a.login.busy {
		t.Fatal("an email address went to the code check")
	}
	a.login.account, a.login.smsCode, a.login.err = "13800001111", "", ""
	a.submitCredentials()
	if a.login.err == "" || a.login.busy {
		t.Fatal("the codeless form went to the network")
	}
}

// A sent code starts the resend wait and the wait blocks another send; a failed send shows why instead.
func TestSendLoginCodeWaitsAndShowsFailure(t *testing.T) {
	a := newTestApp()
	old := sendSMSCode
	defer func() { sendSMSCode = old }()
	sent := make(chan string, 2)
	sendSMSCode = func(_ context.Context, phone string) error {
		sent <- phone
		return nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1, credMode: 1, account: " 13800001111"}
	a.sendLoginCode()
	if got := wait(t, sent, "the code send"); got != "13800001111" {
		t.Fatalf("the send got %q, not the trimmed phone", got)
	}
	if !a.login.smsBusy {
		t.Fatal("the in-flight send did not hold the button busy")
	}
	a.applyLoginCode(1, nil)
	if a.login.smsBusy || a.loginSMSLeft() == 0 {
		t.Fatal("the sent code did not start the resend wait")
	}
	a.sendLoginCode()
	select {
	case phone := <-sent:
		t.Fatalf("the resend wait let another send through for %s", phone)
	case <-time.After(50 * time.Millisecond):
	}
	sendSMSCode = func(context.Context, string) error { return errors.New("发送过于频繁") }
	a.login.smsUntil = time.Time{}
	a.sendLoginCode()
	a.applyLoginCode(1, errors.New("发送过于频繁"))
	if a.login.err != "发送过于频繁" || a.loginSMSLeft() != 0 || a.login.smsBusy {
		t.Fatal("a refused send did not show its reason or wrongly waited")
	}
}

// On the phone the SMS mode's fields type into the code check, which the submit sends.
func TestPhoneSMSLoginFormSubmits(t *testing.T) {
	a := newTestApp()
	a.phone = true
	oldCheck, oldSend, oldPlays := loginBySMS, sendSMSCode, recentPlays
	defer func() { loginBySMS, sendSMSCode, recentPlays = oldCheck, oldSend, oldPlays }()
	recentPlays = func(int) ([]netease.Play, error) { return nil, nil }
	sent := make(chan string, 1)
	sendSMSCode = func(_ context.Context, phone string) error {
		sent <- phone
		return nil
	}
	called := make(chan [2]string, 1)
	loginBySMS = func(_ context.Context, phone, code string) (*netease.Profile, func() error, error) {
		called <- [2]string{phone, code}
		return &netease.Profile{UserID: 42}, func() error { return nil }, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1}
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tapPhone(t, tt, "验证码登录")
	tapPhone(t, tt, "手机号")
	tt.Type("13800001111")
	tapPhone(t, tt, "获取验证码")
	if got := wait(t, sent, "the code send"); got != "13800001111" {
		t.Fatalf("the send got %q, not the typed phone", got)
	}
	a.applyLoginCode(1, nil)
	tapPhone(t, tt, "验证码")
	tt.Type("246810")
	tapPhone(t, tt, "登录")
	if got := wait(t, called, "the code check"); got != [2]string{"13800001111", "246810"} {
		t.Fatalf("the check got %v, not the typed credentials", got)
	}
	a.applyPasswordLogin(1, &netease.Profile{UserID: 42}, func() error { return nil }, nil)
	if a.profile == nil || a.profile.UserID != 42 || !a.login.finished {
		t.Fatal("the accepted code check did not sign in")
	}
}

// On the phone the credentials tab's fields type into the check, which the submit sends.
func TestPhoneLoginFormSubmits(t *testing.T) {
	a := newTestApp()
	a.phone = true
	oldCheck, oldPlays := loginByPassword, recentPlays
	defer func() { loginByPassword, recentPlays = oldCheck, oldPlays }()
	recentPlays = func(int) ([]netease.Play, error) { return nil, nil }
	called := make(chan [2]string, 1)
	loginByPassword = func(_ context.Context, account, password string) (*netease.Profile, func() error, error) {
		called <- [2]string{account, password}
		return &netease.Profile{UserID: 42}, func() error { return nil }, nil
	}
	a.login = loginState{open: true, gen: 1, tab: 1}
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tapPhone(t, tt, "手机号或邮箱")
	tt.Type("13800001111")
	tapPhone(t, tt, "密码")
	tt.Type("secret")
	tapPhone(t, tt, "登录")
	if got := wait(t, called, "the credentials check"); got != [2]string{"13800001111", "secret"} {
		t.Fatalf("the check got %v, not the typed credentials", got)
	}
	a.applyPasswordLogin(1, &netease.Profile{UserID: 42}, func() error { return nil }, nil)
	if a.profile == nil || a.profile.UserID != 42 || !a.login.finished {
		t.Fatal("the accepted check did not sign in")
	}
}

// A failed keychain write leaves the account signed out and the dialog open for retry.
func TestLoginKeepsDialogWhenSessionCannotSave(t *testing.T) {
	a := newTestApp()
	a.login = loginState{open: true, gen: 1, status: netease.QRScanned}
	result := netease.QRLoginStatus{Code: netease.QRConfirmed, Profile: &netease.Profile{UserID: 42}}
	if !a.applyLoginCheck(1, result, nil, func() error { return errors.New("keychain unavailable") }) {
		t.Fatal("polled again after failing to store the session")
	}
	if !a.login.open || a.login.finished || a.login.err == "" || a.profile != nil {
		t.Fatal("reported a signed-in account without a saved session")
	}
}
