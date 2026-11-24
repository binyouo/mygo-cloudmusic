package netease

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

// A message's own JSON names its text, or what it shares; other payloads
// pass through.
func TestMsgText(t *testing.T) {
	for raw, want := range map[string]string{
		`{"msg":"明天见","type":0}`:                          "明天见",
		`{"msg":"","type":1,"song":{"name":"春日影"}}`:       "分享了歌曲《春日影》",
		`{"msg":"","type":2,"playlist":{"name":"宝船"}}`:    "分享了歌单《宝船》",
		`{"msg":"","type":5,"program":{"name":"第一期"}}`:   "分享了节目《第一期》",
		`{"msg":"","type":9,"event":{}}`:                  "分享了动态",
		"还不是一个 JSON":                                    "还不是一个 JSON",
		`{"type":0}`:                                      "…",
	} {
		if got := msgText(raw); got != want {
			t.Errorf("msgText(%s) = %q, want %q", raw, got, want)
		}
	}
}

// unsealForm opens a weapi request's data, the params sealed under the
// request's own key and the preset key.
func unsealForm(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	inner := unseal(t, r.Form.Get("params"), weapiSecret())
	var data map[string]any
	if err := json.Unmarshal(unseal(t, string(inner), weapiPresetKey), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func swapTransport(t *testing.T, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	oldClient, oldTransport := httpClient, http.DefaultTransport
	t.Cleanup(func() { httpClient, http.DefaultTransport = oldClient, oldTransport })
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{Jar: jar}
	http.DefaultTransport = weapiTransport(fn)
	old := weapiSecret
	t.Cleanup(func() { weapiSecret = old })
	weapiSecret = func() string { return "abcdefghijklmnop" }
}

func respond(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

// The conversation list asks the weapi for the threads and names each
// one's other end, whichever side of its last message that is.
func TestPrivateConversations(t *testing.T) {
	var msgs string
	swapTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/weapi/msg/private/users" {
			t.Fatalf("requested %s", r.URL.Path)
		}
		data := unsealForm(t, r)
		if data["offset"].(float64) != 0 || data["limit"].(float64) != 30 {
			t.Fatalf("paged %v %v", data["offset"], data["limit"])
		}
		return respond(msgs), nil
	})
	msgs = `{"code":200,"msgs":[` +
		`{"fromUser":{"userId":9,"nickname":"灯","avatarUrl":"a"},"toUser":{"userId":1,"nickname":"我"},` +
		`"newMsgCount":2,"lastMsg":"{\"msg\":\"明天见\",\"type\":0}","lastMsgTime":1700000000000},` +
		`{"fromUser":{"userId":1,"nickname":"我"},"toUser":{"userId":7,"nickname":"立希"},` +
		`"newMsgCount":0,"lastMsg":"{\"msg\":\"好\",\"type\":0}","lastMsgTime":1690000000000}]}`
	convs, err := PrivateConversations(1, 0, 30)
	if err != nil || len(convs) != 2 {
		t.Fatalf("conversations %v, %v", convs, err)
	}
	if convs[0].Peer.UserID != 9 || convs[0].Peer.Nickname != "灯" || convs[0].Unread != 2 {
		t.Fatalf("first conversation %+v", convs[0])
	}
	if convs[0].LastMsg != "明天见" || convs[0].Time != 1700000000000 {
		t.Fatalf("last message %+v", convs[0])
	}
	if convs[1].Peer.UserID != 7 || convs[1].Peer.Nickname != "立希" {
		t.Fatalf("second conversation %+v", convs[1])
	}
}

// History asks for the messages with one user before a time, and reads
// the text each message's own JSON carries.
func TestPrivateHistory(t *testing.T) {
	swapTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/weapi/msg/private/history" {
			t.Fatalf("requested %s", r.URL.Path)
		}
		data := unsealForm(t, r)
		if data["userId"] != "9" || data["time"].(float64) != 1690000000000 {
			t.Fatalf("asked for %v before %v", data["userId"], data["time"])
		}
		return respond(`{"code":200,"msgs":[` +
			`{"fromUser":{"userId":9,"nickname":"灯"},"toUser":{"userId":1},"msg":"{\"msg\":\"你好\",\"type\":0}","time":1700000000000},` +
			`{"fromUser":{"userId":1},"toUser":{"userId":9},"msg":"{\"msg\":\"在吗\",\"type\":0}","time":1699000000000}]}`), nil
	})
	msgs, err := PrivateHistory(9, 1690000000000, 30)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("history %v, %v", msgs, err)
	}
	if msgs[0].From.UserID != 9 || msgs[0].Text != "你好" || msgs[0].Time != 1700000000000 {
		t.Fatalf("message %+v", msgs[0])
	}
}

// Sending seals the recipient's ID and the text, and reads back the
// message NetEase recorded; a refusal fails the send.
func TestSendPrivate(t *testing.T) {
	var body string
	swapTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/weapi/msg/private/send" {
			t.Fatalf("requested %s", r.URL.Path)
		}
		data := unsealForm(t, r)
		if data["userIds"] != "[9]" || data["msg"] != "晚上好" || data["type"] != "text" {
			t.Fatalf("sent %v", data)
		}
		return respond(body), nil
	})
	body = `{"code":200,"newMsgs":[{"fromUser":{"userId":1},"toUser":{"userId":9},"msg":"{\"msg\":\"晚上好\",\"type\":0}","time":1700000000001}]}`
	m, err := SendPrivate(9, "晚上好")
	if err != nil || m.Text != "晚上好" || m.From.UserID != 1 || m.Time != 1700000000001 {
		t.Fatalf("sent %+v, %v", m, err)
	}
	body = `{"code":415,"message":"refused"}`
	if m, err := SendPrivate(9, "晚上好"); err == nil || m != nil {
		t.Fatal("a refused message was sent")
	}
	// NetEase answering none still has the message sent.
	body = `{"code":200,"newMsgs":[]}`
	m, err = SendPrivate(9, "晚上好")
	if err != nil || m == nil || m.Text != "晚上好" || m.Time == 0 {
		t.Fatalf("echo %+v, %v", m, err)
	}
}
