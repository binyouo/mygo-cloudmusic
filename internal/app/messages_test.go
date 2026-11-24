package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Older pages of a chat prepend, keeping one of each message, and new
// ones merge into the order their times give.
func TestChatMergesPages(t *testing.T) {
	ch := &chatState{}
	// History comes newest first.
	ch.reset([]netease.Message{
		{From: netease.MsgUser{UserID: 9}, Text: "二", Time: 200},
		{From: netease.MsgUser{UserID: 1}, Text: "一", Time: 100},
	})
	if len(ch.msgs) != 2 || ch.msgs[0].Text != "一" || ch.msgs[1].Text != "二" {
		t.Fatalf("first page %v", ch.msgs)
	}
	ch.mergeNew([]netease.Message{
		{From: netease.MsgUser{UserID: 9}, Text: "四", Time: 400},
		{From: netease.MsgUser{UserID: 9}, Text: "二", Time: 200}, // a repeat
		{From: netease.MsgUser{UserID: 1}, Text: "三", Time: 250}, // late, between
	})
	if len(ch.msgs) != 4 {
		t.Fatalf("merged %v", ch.msgs)
	}
	for i, want := range []string{"一", "二", "三", "四"} {
		if ch.msgs[i].Text != want {
			t.Fatalf("order %v", ch.msgs)
		}
	}
	ch.prependOlder([]netease.Message{
		{From: netease.MsgUser{UserID: 9}, Text: "零", Time: 50},
		{From: netease.MsgUser{UserID: 1}, Text: "一", Time: 100}, // a repeat
	})
	if len(ch.msgs) != 5 || ch.msgs[0].Text != "零" || ch.msgs[1].Text != "一" {
		t.Fatalf("prepended %v", ch.msgs)
	}
}

// A chat opened without its conversation names the other end from the
// messages it sent.
func TestChatInfersPeer(t *testing.T) {
	ch := &chatState{peer: netease.MsgUser{UserID: 9}}
	ch.mergeNew([]netease.Message{
		{From: netease.MsgUser{UserID: 9, Nickname: "灯"}, Text: "你好", Time: 100},
	})
	ch.inferPeer(9)
	if ch.peer.Nickname != "灯" {
		t.Fatalf("peer %+v", ch.peer)
	}
}

// A conversation row opens its chat and reads its unread count; the chat
// asks for its history and shows its messages.
func TestConversationOpensChat(t *testing.T) {
	asked := make(chan int64, 2)
	oldHistory, oldConvs := privateHistory, privateConversations
	defer func() { privateHistory, privateConversations = oldHistory, oldConvs }()
	privateConversations = func(mine int64, offset, limit int) ([]netease.Conversation, error) {
		return nil, nil
	}
	privateHistory = func(uid, before int64, limit int) ([]netease.Message, error) {
		asked <- uid
		return nil, nil
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "我"}
	a.msgs = convsState{convs: []netease.Conversation{{
		Peer: netease.MsgUser{UserID: 9, Nickname: "灯"}, LastMsg: "明天见", Time: 1700000000000, Unread: 2,
	}}}
	a.msgs.loaded = true
	a.route = route{page: pageMessages}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("灯"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageChat, id: 9}) {
		t.Fatalf("opened %v", a.route)
	}
	if a.msgs.convs[0].Unread != 0 {
		t.Fatal("the thread kept its unread count")
	}
	if uid := wait(t, asked, "the chat's history"); uid != 9 {
		t.Fatalf("asked for the history of %d", uid)
	}
	ch := a.chats[9]
	ch.reset([]netease.Message{
		{From: netease.MsgUser{UserID: 9, Nickname: "灯"}, Text: "在吗", Time: 100},
		{From: netease.MsgUser{UserID: 1}, Text: "在", Time: 200},
	})
	ch.loaded = true
	tt.Frame()
	if _, ok := tt.Find("在吗"); !ok {
		t.Fatal("the peer's message does not show")
	}
}

// Enter in a chat's field sends its draft; the send asks NetEase with the
// other end's ID.
func TestChatSendsOnEnter(t *testing.T) {
	sent := make(chan string, 2)
	oldSend, oldHistory := sendPrivate, privateHistory
	defer func() { sendPrivate, privateHistory = oldSend, oldHistory }()
	privateHistory = func(uid, before int64, limit int) ([]netease.Message, error) {
		return nil, nil
	}
	sendPrivate = func(uid int64, text string) (*netease.Message, error) {
		sent <- text
		return &netease.Message{From: netease.MsgUser{UserID: 1}, Text: text, Time: 300}, nil
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "我"}
	ch := a.chatFor(9, netease.MsgUser{UserID: 9, Nickname: "灯"})
	ch.loaded = true
	a.open(route{page: pageChat, id: 9})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("私信内容"); err != nil {
		t.Fatal(err)
	}
	tt.Type("晚上好")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if text := wait(t, sent, "the message"); text != "晚上好" {
		t.Fatalf("sent %q", text)
	}
	if !ch.sending {
		t.Fatal("the send did not start")
	}
}
