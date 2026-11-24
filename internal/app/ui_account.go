package app

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"rsc.io/qr"

	"netease-music-mygo/internal/netease"
)

// loginState is a login dialog under way: the QR attempt on tab 0, the
// credentials form on tab 1.
type loginState struct {
	open     bool
	gen      int // counts logins, to stop the polls of closed ones
	cancel   context.CancelFunc
	poll     *loginPoll
	code     *qr.Code
	status   int // a QR state, or 0 while the code loads
	scanner  string
	avatar   string
	err      string
	finished bool
	tab      int // 0 = 扫码登录, 1 = 账号密码登录
	credMode int // the credentials tab: 0 = 密码, 1 = 短信验证码
	account  string
	password string
	smsCode  string
	smsBusy  bool      // a code send is in flight
	smsUntil time.Time // the resend wait ends at
	busy     bool      // a credentials check is in flight
}

// One worker keeps the QR attempt across foreground changes. A change
// wakes it without replacing the attempt or starting a second request.
type loginPoll struct {
	active atomic.Bool
	wake   chan struct{}
}

func (p *loginPoll) wait(ctx context.Context, tick <-chan time.Time) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-p.wake:
		case <-tick:
		}
		if p.active.Load() {
			return ctx.Err() == nil
		}
	}
}

// boot signs in with the session in the keychain, if any, then loads the
// home page for whoever is listening.
func (a *app) boot() {
	a.homeLoading = true
	a.accountRestorePending = true
	a.restoreAccount()
}

func (a *app) restoreAccount() {
	if !a.accountRestorePending || a.accountRestoreInFlight || a.sessionChanging {
		return
	}
	a.accountRestoreInFlight = true
	revision := a.accountRevision
	go func() {
		account, err := netease.LoadSavedAccount(context.Background())
		a.post(func() {
			a.accountRestoreInFlight = false
			a.applyRestoredAccount(revision, account, err)
		})
	}()
}

func (a *app) applyRestoredAccount(revision uint64, account *netease.SavedAccount, err error) {
	if revision != a.accountRevision {
		a.restoreAccount()
		return
	}
	if err != nil {
		// Keep the saved session for the next foreground attempt. Still load
		// the guest home and the local queue if startup was waiting for it.
		if a.homeLoading && a.homeAt.IsZero() {
			a.signedIn(nil)
		}
		return
	}
	a.accountRestorePending = false
	var profile *netease.Profile
	if account != nil {
		a.sessionChanging = true
		err := account.Activate()
		a.sessionChanging = false
		if err != nil {
			a.toast = "无法清除过期的登录会话"
		} else {
			profile = account.Profile
		}
	}
	a.signedIn(profile)
}

// signedIn shows what is personal to prof, or to nobody.
func (a *app) signedIn(prof *netease.Profile) {
	a.accountRevision++
	a.profile = prof
	a.resetTrackComments()
	a.phoneTrackTarget, a.phoneAddTo = nil, nil
	a.phonePlaylistTarget = 0
	a.radioStarting = nil
	a.daily, a.picked = nil, pickedUp{}
	a.likeSync.request, a.likeSync.changes, a.likeSync.pins = nil, nil, nil
	if a.likeSync.verify != nil {
		a.likeSync.verify.Stop()
		a.likeSync.verify = nil
	}
	a.likeSync.revision++
	// Playback picks up where it was left, once the session that lets
	// the listener play their songs is back.
	a.restoreSession()
	a.mine, a.saved, a.recPlaylists, a.liked = nil, nil, nil, nil
	a.likedPrograms, a.askedPrograms = nil, nil
	// What the listener may play, and follows, changed: load the pages
	// again.
	a.playlists = map[int64]*netease.Playlist{}
	a.loadingPl = map[int64]bool{}
	a.plErr = map[int64]string{}
	a.playlistMore, a.playlistFilter = nil, songFilter{}
	a.artists, a.albums, a.users, a.podcasts = map[int64]*artistData{}, map[int64]*albumData{}, map[int64]*userData{}, map[int64]*podcastData{}
	a.podcastTab = &podcastTab{}
	a.searches = map[route]*searchData{}
	a.msgs, a.chats = convsState{}, map[int64]*chatState{}
	a.load(a.route)
	a.loadHome()
	if prof == nil {
		return
	}
	a.loadLiked(prof.UserID)
	a.loadMine(prof.UserID)
}

func (a *app) openLogin() {
	if a.sessionChanging {
		return
	}
	a.stopLoginPolling()
	a.login = loginState{open: true, gen: a.login.gen + 1, tab: a.login.tab, credMode: a.login.credMode}
	if a.login.tab == 0 {
		a.beginQR()
	}
}

// beginQR starts the QR attempt of the current login generation: the
// code, then a foreground poll every two seconds until it ends.
func (a *app) beginQR() {
	ctx, cancel := context.WithCancel(context.Background())
	poll := &loginPoll{wake: make(chan struct{}, 1)}
	poll.active.Store(!a.phoneSuspended)
	l := &a.login
	l.cancel, l.poll = cancel, poll
	l.code, l.status, l.err = nil, 0, ""
	gen := l.gen
	go func() {
		attempt, err := netease.BeginQRLogin(ctx)
		var code *qr.Code
		if err == nil {
			code, err = qr.Encode(attempt.URL(), qr.M)
		}
		a.post(func() {
			if gen != a.login.gen || !a.login.open {
				return
			}
			if err != nil {
				a.login.err = "无法获取二维码，请检查网络"
				return
			}
			a.login.code, a.login.status = code, netease.QRWaiting
			a.pollLogin(ctx, gen, attempt, poll)
		})
	}()
}

func (a *app) stopLoginPolling() {
	if a.login.cancel != nil {
		a.login.cancel()
		a.login.cancel, a.login.poll = nil, nil
	}
}

func (a *app) setLoginPollingActive(active bool) {
	poll := a.login.poll
	if !a.login.open || poll == nil {
		return
	}
	poll.active.Store(active)
	select {
	case poll.wake <- struct{}{}:
	default:
	}
}

// switchLoginTab moves the dialog between the QR code and the credentials
// form: the QR poll stops while the form shows and begins anew on return.
func (a *app) switchLoginTab(tab int) {
	l := &a.login
	if tab == l.tab || !l.open {
		return
	}
	l.tab, l.err = tab, ""
	if tab == 0 && l.poll == nil {
		a.beginQR()
	} else if tab != 0 {
		a.stopLoginPolling()
	}
}

// loginByPassword is the credentials check, a variable so tests swap it:
// the profile signed in, the commit of the session it earned, or why not.
var loginByPassword = func(ctx context.Context, account, password string) (*netease.Profile, func() error, error) {
	attempt, err := netease.LoginWithPassword(ctx, account, password)
	if err != nil {
		return nil, nil, err
	}
	return attempt.Profile, attempt.Commit, nil
}

// loginBySMS is the SMS code check, a variable so tests swap it: the
// profile signed in, the commit of the session it earned, or why not.
var loginBySMS = func(ctx context.Context, phone, code string) (*netease.Profile, func() error, error) {
	attempt, err := netease.LoginWithSMSCode(ctx, phone, code)
	if err != nil {
		return nil, nil, err
	}
	return attempt.Profile, attempt.Commit, nil
}

// sendSMSCode texts a login code to a phone, a variable so tests swap it.
var sendSMSCode = func(ctx context.Context, phone string) error {
	return netease.SendSMSCode(ctx, phone)
}

// submitCredentials sends the credentials form as its mode says: the
// password, or the SMS code the listener was texted.
func (a *app) submitCredentials() {
	if a.login.credMode == 1 {
		a.submitSMSLogin()
		return
	}
	a.submitPasswordLogin()
}

// submitPasswordLogin signs in with the form's phone or email and its
// password; the result lands through applyPasswordLogin.
func (a *app) submitPasswordLogin() {
	l := &a.login
	if l.busy || a.sessionChanging || !l.open || l.finished {
		return
	}
	account, password := strings.TrimSpace(l.account), l.password
	switch {
	case account == "":
		l.err = "输入手机号或邮箱"
		return
	case password == "":
		l.err = "输入密码"
		return
	}
	l.busy, l.err = true, ""
	gen := l.gen
	go func() {
		profile, commit, err := loginByPassword(context.Background(), account, password)
		a.post(func() { a.applyPasswordLogin(gen, profile, commit, err) })
	}()
}

// submitSMSLogin signs a phone in with the code NetEase texted it, the
// way past the captcha a bare password hits; the result lands through
// applyPasswordLogin like the password check's.
func (a *app) submitSMSLogin() {
	l := &a.login
	if l.busy || a.sessionChanging || !l.open || l.finished {
		return
	}
	phone, code := strings.TrimSpace(l.account), strings.TrimSpace(l.smsCode)
	switch {
	case !netease.MobileNumber(phone):
		l.err = "验证码登录需要手机号"
		return
	case code == "":
		l.err = "输入短信验证码"
		return
	}
	l.busy, l.err = true, ""
	gen := l.gen
	go func() {
		profile, commit, err := loginBySMS(context.Background(), phone, code)
		a.post(func() { a.applyPasswordLogin(gen, profile, commit, err) })
	}()
}

// sendLoginCode texts a login code to the form's phone; the wait the
// send earns guards the resend.
func (a *app) sendLoginCode() {
	l := &a.login
	if l.smsBusy || l.busy || !l.open || l.finished || a.loginSMSLeft() > 0 {
		return
	}
	phone := strings.TrimSpace(l.account)
	if !netease.MobileNumber(phone) {
		l.err = "验证码登录需要手机号"
		return
	}
	l.smsBusy, l.err = true, ""
	gen := l.gen
	go func() {
		err := sendSMSCode(context.Background(), phone)
		a.post(func() { a.applyLoginCode(gen, err) })
	}()
}

// applyLoginCode takes the send's result: a sent code starts the resend
// wait, a refusal shows why.
func (a *app) applyLoginCode(gen int, err error) {
	l := &a.login
	if gen != l.gen || !l.open {
		return
	}
	l.smsBusy = false
	if err != nil {
		l.err = err.Error()
		return
	}
	l.smsUntil = time.Now().Add(60 * time.Second)
}

// loginSMSLeft is the seconds before another code can be sent.
func (a *app) loginSMSLeft() int {
	if left := time.Until(a.login.smsUntil); left > 0 {
		return int(math.Ceil(left.Seconds()))
	}
	return 0
}

// applyPasswordLogin accepts the credentials check's result: on success it
// commits the session and signs in, as a QR confirmation does. A stale
// generation or a closed dialog discards it.
func (a *app) applyPasswordLogin(gen int, profile *netease.Profile, commit func() error, err error) {
	l := &a.login
	if gen != l.gen || !l.open {
		return
	}
	l.busy = false
	if err != nil {
		l.err = err.Error()
		if l.credMode == 0 && netease.MobileNumber(l.account) && strings.Contains(l.err, "验证") {
			l.err += "，可改用短信验证码"
		}
		return
	}
	if a.sessionChanging {
		l.err = "登录状态切换中，请重试"
		return
	}
	a.sessionChanging = true
	a.accountRevision++
	cerr := commit()
	a.sessionChanging = false
	if cerr != nil {
		l.err = "无法保存登录会话：" + cerr.Error()
		a.refreshAfterSessionError()
		return
	}
	a.accountRestorePending = false
	l.finished = true
	l.password, l.smsCode = "", ""
	a.signedIn(profile)
	a.pickUp(profile.UserID)
	a.closeLoginSoon(gen)
}

// pollLogin checks every two seconds in the foreground until the login
// ends or the dialog closes. Returning to the app checks immediately.
func (a *app) pollLogin(ctx context.Context, gen int, attempt *netease.QRLogin, poll *loginPoll) {
	go func() {
		const interval = 2 * time.Second
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for poll.wait(ctx, timer.C) {
			timer.Stop()
			status, err := attempt.Check(ctx)
			done := make(chan bool, 1)
			a.post(func() {
				done <- a.applyLoginCheck(gen, status, err, attempt.Commit)
			})
			select {
			case <-ctx.Done():
				return
			case finished := <-done:
				if finished {
					return
				}
			}
			timer.Reset(interval)
		}
	}()
}

func (a *app) applyLoginCheck(gen int, status netease.QRLoginStatus, err error, commit func() error) bool {
	l := &a.login
	if gen != l.gen || !l.open {
		return true
	}
	if err != nil {
		// Retry the profile lookup if NetEase already confirmed the scan.
		if status.Code == netease.QRConfirmed {
			l.status = status.Code
		}
		return false
	}
	l.status = status.Code
	if status.Nickname != "" {
		l.scanner, l.avatar = status.Nickname, status.AvatarURL
	}
	switch status.Code {
	case netease.QRConfirmed:
		if a.sessionChanging {
			return false
		}
		a.sessionChanging = true
		a.accountRevision++
		err := commit()
		a.sessionChanging = false
		if err != nil {
			l.err = "无法保存登录会话，请刷新二维码后重试"
			a.refreshAfterSessionError()
			return true
		}
		a.accountRestorePending = false
		l.finished = true
		a.signedIn(status.Profile)
		a.pickUp(status.Profile.UserID)
		a.closeLoginSoon(gen)
	case netease.QRWaiting, netease.QRScanned:
		return false
	case netease.QRExpired:
		l.err = status.Message
	default:
		l.err = status.Message
		if l.err == "" {
			l.err = "登录失败，请稍后再试"
		}
	}
	return true
}

// closeLoginSoon closes the dialog a moment after it showed the welcome.
func (a *app) closeLoginSoon(gen int) {
	go func() {
		time.Sleep(1200 * time.Millisecond)
		a.post(func() {
			if gen == a.login.gen {
				a.login.open = false
				a.stopLoginPolling()
			}
		})
	}()
}

func (a *app) signOut() {
	if a.sessionChanging {
		return
	}
	a.stopLoginPolling()
	a.login.open = false
	a.login.gen++
	a.sessionChanging = true
	a.accountRevision++
	go func() {
		revoke, err := netease.EndSession()
		a.post(func() {
			a.sessionChanging = false
			if err != nil {
				a.toast = "无法清除登录会话，请稍后重试"
				a.refreshAfterSessionError()
				return
			}
			a.accountRestorePending = false
			a.signedIn(nil)
		})
		if revoke != nil {
			revoke()
		}
	}()
}

func (a *app) refreshAfterSessionError() {
	if a.accountRestorePending {
		a.restoreAccount()
	} else if a.homeLoading {
		a.loadHome()
	}
}

// shareLoginCode is available when the host has an iOS share sheet.
var shareLoginCode func(*qr.Code) error

// loginDialog shows the QR code to scan with the NetEase app on a phone.
func (a *app) loginDialog(c *ui.Context) {
	p := a.pal
	l := &a.login
	ui.DialogBase(c.Key("login"), &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		w, h := c.Size()
		qrSize := float32(212)
		if a.phone {
			top, right, bottom, left := phoneSafeArea(c)
			w -= left + right
			qrSize = min(qrSize, max(124, h-top-bottom-192))
		}
		panel.Width(min(340, max(0, w-24))).Padding(28, 28, 26, 28).Radius(22).Gap(6).AlignItems(ui.Center).
			Background(p.bg).Border(1, p.hairline)
		if a.phone {
			panel.Background(p.sidebar).TextColor(p.text)
			iconButton(c.Key("close-login"), iconX, 16, p.muted, "关闭登录").Size(44, 44).
				Absolute().Top(8).Right(8).OnClick(func() { l.open = false })
		} else {
			panel.Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
				Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		}
		title, sub := "扫码登录", "打开网易云音乐 App，扫一扫登录"
		if l.tab == 1 {
			title, sub = "账号登录", "手机号或邮箱，加账号密码"
			if l.credMode == 1 {
				sub = "输入手机号，用短信验证码登录"
			}
		}
		switch {
		case l.finished:
			title, sub = "登录成功", "欢迎回来，"+a.profileName()
		case l.tab == 1:
			// The form keeps its own subtitle; errors show under its fields.
		case l.err != "":
			sub = l.err
		case l.status == netease.QRScanned:
			title, sub = "扫描成功", "请在手机上确认登录"
		case l.status == netease.QRConfirmed:
			title, sub = "已确认登录", "正在读取账户信息…"
		case l.status == netease.QRExpired:
			sub = "二维码已失效"
		case a.phone && shareLoginCode != nil:
			sub = "在网易云音乐的扫一扫里扫码，或保存后从相册选取"
		}
		ui.Text(c, title).FontSize(20).FontWeight(700)
		ui.Text(c, sub).FontSize(13).TextColor(p.muted).TextAlign(ui.Center).Margin(0, 0, 12, 0)

		if !l.finished {
			ui.Row(c).AlignSelf(ui.Center).Margin(0, 0, 14, 0).Children(func() {
				tab := l.tab
				if segmented(c.Key("login-tabs"), &tab, "扫码登录", "账号密码") {
					a.switchLoginTab(tab)
				}
			})
		}
		if l.tab == 1 {
			a.loginForm(c, l)
			return
		}

		ui.Box(c).Size(qrSize, qrSize).Radius(16).Background(ui.RGB(255, 255, 255)).Border(1, p.hairline).Children(func() {
			if l.code != nil {
				code := l.code
				opacity := float32(1)
				if l.status != netease.QRWaiting || l.err != "" {
					opacity = 0.12
				}
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Opacity(opacity).
					Draw(func(pt *ui.Painter, r ui.Rect) { drawQR(pt, r, code, 14) })
			} else if l.err == "" {
				// On white whatever the appearance, as the code is.
				s := shimmer{base: ui.RGBA(0, 0, 0, 0.08), sheen: ui.RGBA(0, 0, 0, 0.03)}.in(c)
				qrSkeleton(c, s, 14).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			}
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(10).Children(func() {
				switch {
				case l.finished || l.status == netease.QRScanned || l.status == netease.QRConfirmed && l.err == "":
					if bmp := bitmap(a.art.get(netease.Sized(l.avatar, 160), false)); bmp != nil {
						pic := ui.Box(c).Size(72, 72).Radius(36).Shadow(0, 6, 16, -4, ui.RGBA(0, 0, 0, 0.3))
						in := artIn(pic, bmp)
						pic.Children(func() { artImage(c, bmp, 36, in) })
					}
					if l.scanner != "" {
						ui.Text(c, l.scanner).FontSize(14).FontWeight(600).TextColor(ui.Hex("#1d1d1f"))
					}
				case l.status == netease.QRExpired || l.err != "":
					if pillButton(c.Key("refresh"), iconRepeat, "刷新二维码", p.accent, p.onAccent).Clicked() {
						a.openLogin()
					}
				}
			})
		})
		if code := l.code; shareLoginCode != nil && code != nil && l.status == netease.QRWaiting {
			a.settingButton(c, "save-qr", "保存登录二维码", false).MinHeight(44).OnClick(func() {
				go func() {
					if err := shareLoginCode(code); err != nil {
						a.post(func() { a.toast = "无法保存二维码：" + err.Error() })
					}
				}()
			})
		}
		if !a.phone {
			ui.Text(c, "登录后，每日推荐与歌单将根据你的口味生成").FontSize(11.5).TextColor(p.faint).Margin(16, 0, 0, 0)
		}
	})
	if !l.open {
		a.stopLoginPolling()
	}
}

// loginForm is the credentials tab of the login dialog: phone or email,
// the password or the texted code, and the submit; NetEase's error lands
// under the fields.
func (a *app) loginForm(c *ui.Context, l *loginState) {
	p := a.pal
	ui.Column(c).Gap(10).AlignSelf(ui.Stretch).Children(func() {
		ui.Row(c).AlignSelf(ui.Center).Margin(0, 0, 4, 0).Children(func() {
			mode := l.credMode
			if segmented(c.Key("cred-mode"), &mode, "密码登录", "验证码登录") {
				l.credMode, l.err = mode, ""
			}
		})
		if l.credMode == 1 {
			a.loginField(c, "account", &l.account, &a.loginAccountBox, "手机号", "手机号", false, a.submitCredentials, ui.KeyboardPhone)
			a.loginCodeRow(c, l)
			return
		}
		a.loginField(c, "account", &l.account, &a.loginAccountBox, "手机号或邮箱", "手机号或邮箱", false, nil, ui.KeyboardDefault)
		a.loginField(c, "password", &l.password, &a.loginPasswordBox, "密码", "密码", true, a.submitCredentials, ui.KeyboardDefault)
	})
	if l.err != "" {
		ui.Text(c, l.err).FontSize(12.5).TextColor(p.accent).TextAlign(ui.Center).Margin(10, 0, 0, 0)
	}
	label := "登录"
	if l.busy {
		label = "正在登录…"
	}
	b := ui.ButtonBase(c.Key("submit")).Height(44).Radius(10).Center().Margin(14, 0, 0, 0).AlignSelf(ui.Stretch).
		Label(label).Disabled(l.busy).OnClick(a.submitCredentials)
	bg := p.accent
	if l.busy {
		bg = bg.Alpha(0.6)
	}
	b.Background(bg)
	b.Children(func() { ui.Text(c, label).FontSize(15).FontWeight(600).TextColor(p.onAccent).SingleLine() })
}

// loginCodeRow is the SMS half of the credentials form: the code field,
// and beside it the send button with its resend wait.
func (a *app) loginCodeRow(c *ui.Context, l *loginState) {
	p := a.pal
	ui.Row(c).Gap(8).Children(func() {
		a.loginField(c, "code", &l.smsCode, &a.loginCodeBox, "短信验证码", "验证码", false, a.submitCredentials, ui.KeyboardNumber).Grow(1).MinWidth(0)
		left := a.loginSMSLeft()
		label := "获取验证码"
		switch {
		case l.smsBusy:
			label = "发送中…"
		case left > 0:
			label = fmt.Sprintf("%ds", left)
			c.After(300 * time.Millisecond)
		case !l.smsUntil.IsZero():
			label = "重新发送"
		}
		disabled := l.smsBusy || l.busy || left > 0
		b := ui.ButtonBase(c.Key("send-code")).Height(44).PaddingX(14).Radius(10).Center().
			Label("获取验证码").Disabled(disabled).Background(p.bg).Border(1, p.hairline).OnClick(a.sendLoginCode)
		tc := p.accent
		if disabled {
			tc = p.muted
		}
		b.Children(func() { ui.Text(c, label).FontSize(13).TextColor(tc).SingleLine() })
	})
}

// loginField is one credentials input in a quiet pill, focused on tap.
func (a *app) loginField(c *ui.Context, key string, value *string, handle *ui.Handle, placeholder, label string, password bool, onSubmit func(), keyboard ui.KeyboardType) ui.Element {
	p := a.pal
	field := ui.Row(c.Key(key)).Height(44).PaddingX(12).Radius(10).AlignItems(ui.Center).Cursor(ui.CursorText).
		OnClick(func() { handle.Focus(c) })
	if c.Focused(*handle) {
		field.Background(p.bg).Border(1, p.accent.Alpha(0.6)).Shadow(0, 0, 0, 3, p.accent.Alpha(0.22))
	} else {
		field.Background(p.bg).Border(1, p.hairline)
	}
	field.Children(func() {
		in := ui.TextInputBase(c.Key("in"), value).Bind(handle).Grow(1).MinWidth(0).FillHeight().
			FontSize(14).Keyboard(keyboard).Placeholder(placeholder).Label(label).TextColor(p.text)
		if password {
			in.Password()
		}
		if in.Submitted() && onSubmit != nil {
			onSubmit()
		}
	})
	return field
}

// drawQR draws a QR code in r, as crisp squares on white, inside a quiet
// zone of pad.
func drawQR(p *ui.Painter, r ui.Rect, code *qr.Code, pad float32) {
	n := code.Size
	cell := (min(r.W, r.H) - 2*pad) / float32(n)
	x0 := r.X + (r.W-cell*float32(n))/2
	y0 := r.Y + (r.H-cell*float32(n))/2
	ink := ui.Hex("#141416")
	for y := range n {
		for x := range n {
			if code.Black(x, y) {
				// A hair larger, so neighbors join without seams.
				p.Fill(ui.Rect{X: x0 + float32(x)*cell, Y: y0 + float32(y)*cell, W: cell + 0.3, H: cell + 0.3}, ink, 0)
			}
		}
	}
}

func (a *app) profileName() string {
	if a.profile == nil {
		return ""
	}
	return a.profile.Nickname
}

// accountRow sits at the foot of the sidebar: who is signed in, with a
// dropdown to open their page or the settings or sign out, or a way to
// sign in.
func (a *app) accountRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("account")).Height(40).PaddingX(8).Gap(10).Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || a.accountOpen && a.profile != nil {
		b.Background(p.hover)
	}
	if a.profile == nil {
		a.accountOpen = false
		b.Label("登录").Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
			ui.Text(c, "登录网易云音乐").FontSize(13).FontWeight(500).TextColor(p.muted)
		})
		if b.Clicked() {
			a.openLogin()
		}
		return
	}
	prof := a.profile
	b.Label(prof.Nickname).Expanded(a.accountOpen).Children(func() {
		a.avatar(c, prof.AvatarURL, 26, p.press)
		ui.Text(c, prof.Nickname).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountMenu drops up from the account row, as wide as it, in the colors
// of the volume popover: the listener's own page and the settings, then
// signing out.
func (a *app) accountMenu(c *ui.Context, row ui.Element) {
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("account-menu"), row, &a.accountOpen, func(panel ui.Element) {
		panel.AttachTo(row, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户")
		if w := row.Bounds().W; w > 0 {
			panel.Width(w)
		}
		shortcut := "Ctrl+,"
		if runtime.GOOS == "darwin" {
			shortcut = "⌘,"
		}
		if a.menuItem(c, iconUser, "我的主页", "").AutoFocus().Clicked() {
			a.accountOpen = false
			a.open(route{page: pageUser, id: a.profile.UserID})
		}
		if a.menuItem(c, iconSettings, "设置…", shortcut).Clicked() {
			a.accountOpen = false
			a.openSettings()
		}
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
		if a.menuItem(c, iconSignOut, "退出登录", "").Clicked() {
			a.accountOpen = false
			a.signOut()
		}
	})
}

// menuItem is an item of a dropdown: an icon, its label, and the shortcut
// that does the same, if any.
func (a *app) menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// myPlaylists lists the listener's playlists under a header that opens
// and closes them: those they made, if mine, with a button to make one
// and a menu to rename them, or else with a menu to unsave them.
func (a *app) myPlaylists(c *ui.Context, title string, lists []netease.Playlist, mine bool, collapsed *bool) {
	if len(lists) == 0 {
		return
	}
	p := a.pal
	headerHeight, rowHeight, coverSize := float32(26), float32(34), float32(22)
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(18, 4, 6, 6).Children(func() {
		b := ui.ButtonBase(c.Key("toggle")).Grow(1).Height(headerHeight).PaddingX(4).Gap(6).Radius(6).
			AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleDisclosure).Label(title)
		if b.Clicked() {
			*collapsed = !*collapsed
		}
		b.Expanded(!*collapsed)
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
			arrow := iconDown
			if *collapsed {
				arrow = iconForward
			}
			ui.Icon(c, arrow).Size(12, 12).TextColor(p.faint)
		})
		if mine && iconButton(c.Key("new"), iconPlus, 13, p.faint, "新建歌单").Size(headerHeight, headerHeight).Radius(6).Clicked() {
			a.newPlaylist(nil)
		}
	})
	if *collapsed {
		return
	}
	for i := range lists {
		pl := &lists[i]
		selected := a.route.page == pagePlaylist && a.route.id == pl.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(pl.ID)).Height(rowHeight).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		switch {
		case selected:
			b.Background(p.press)
		case b.Hovered():
			b.Background(p.hover)
		}
		canManage := !mine || pl.SpecialType != 5
		if canManage {
			b.ContextMenu(a.playlistManagementItems(pl, mine))
		}
		b.Children(func() {
			a.cover(c, pl.Cover(), 80, coverSize, 5)
			ui.Text(c, pl.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		})
		if b.Clicked() {
			a.open(route{page: pagePlaylist, id: pl.ID})
		}
	}
}
