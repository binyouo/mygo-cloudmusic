package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// These load the listener's conversations and a chat's messages, and send
// one, with NetEase; tests replace them.
var (
	privateConversations = netease.PrivateConversations
	privateHistory       = netease.PrivateHistory
	sendPrivate          = netease.SendPrivate
)

// convsState is the private-message list: the listener's threads, a page
// at a time as they scroll.
type convsState struct {
	convs []netease.Conversation
	paged
}

// chatState is one thread: its messages, oldest first, its draft, the
// list that shows them, and the poll keeping them current while open.
type chatState struct {
	peer       netease.MsgUser
	msgs       []netease.Message
	paged
	draft      string
	sending    bool
	sendErr    string
	list       ui.ListState
	polling    bool
	refreshing bool // a poll's page is on its way
}

const (
	convPageSize = 50 // the conversations a page loads
	chatPageSize = 30 // the messages a page of a chat loads
	// chatPoll is how often the open chat asks for its new messages.
	chatPoll = 8 * time.Second
	// chatTimeGap is the pause between messages after which one shows
	// its time.
	chatTimeGap = 5 * 60 * 1000
)

// msgKey identifies a message for its list: its time and sender, which
// name it as nothing else does.
type msgKey struct {
	time, from int64
}

func keyOf(m netease.Message) msgKey { return msgKey{m.Time, m.From.UserID} }

// chatFor is the thread with uid, made on first open with peer, which the
// conversation that opened it knows.
func (a *app) chatFor(uid int64, peer netease.MsgUser) *chatState {
	ch := a.chats[uid]
	if ch == nil {
		ch = &chatState{peer: peer}
		ch.list.FollowEnd = true
		ch.list.Key = func(i int) any {
			if i == 0 {
				return "head"
			}
			return keyOf(ch.msgs[i-1])
		}
		a.chats[uid] = ch
	} else if peer.Nickname != "" {
		// peerOf's bare ID would otherwise replace a name the messages
		// already gave.
		ch.peer = peer
	}
	return ch
}

func (a *app) loadConversations() {
	ms := &a.msgs
	if ms.loading {
		return
	}
	ms.loading, ms.err = true, ""
	fetch, mine, offset := privateConversations, int64(0), len(ms.convs)
	if a.profile != nil {
		mine = a.profile.UserID
	}
	go func() {
		convs, err := fetch(mine, offset, convPageSize)
		a.post(func() {
			ms.loading = false
			if err != nil {
				ms.err = "私信加载失败"
				return
			}
			ms.loaded, ms.more = true, len(convs) >= convPageSize
			seen := make(map[int64]bool, len(ms.convs)+len(convs))
			for _, cv := range ms.convs {
				seen[cv.Peer.UserID] = true
			}
			for _, cv := range convs {
				if !seen[cv.Peer.UserID] {
					seen[cv.Peer.UserID] = true
					ms.convs = append(ms.convs, cv)
				}
			}
		})
	}()
}

// loadChat loads the chat's first page, or, with older, its next older
// page, whose beforetime is its oldest message's time.
func (a *app) loadChat(ch *chatState, uid int64, older bool) {
	if ch.loading {
		return
	}
	var before int64
	if older && len(ch.msgs) > 0 {
		before = ch.msgs[0].Time
	}
	ch.loading, ch.err = true, ""
	fetch := privateHistory
	go func() {
		msgs, err := fetch(uid, before, chatPageSize)
		a.post(func() {
			ch.loading = false
			if err != nil {
				ch.err = "消息加载失败"
				return
			}
			ch.loaded = true
			if len(msgs) >= chatPageSize {
				ch.more = true
			}
			if older {
				ch.prependOlder(msgs)
			} else {
				ch.reset(msgs)
			}
			ch.inferPeer(uid)
		})
	}()
}

// reset replaces the chat's messages with page, which comes newest first.
func (ch *chatState) reset(page []netease.Message) {
	ch.msgs = nil
	ch.mergeNew(page)
}

// prependOlder puts page, which comes newest first, before the messages
// shown, keeping one of each.
func (ch *chatState) prependOlder(page []netease.Message) {
	seen := make(map[msgKey]bool, len(ch.msgs)+len(page))
	for _, m := range ch.msgs {
		seen[keyOf(m)] = true
	}
	var older []netease.Message
	for i := len(page) - 1; i >= 0; i-- {
		if m := page[i]; !seen[keyOf(m)] {
			seen[keyOf(m)] = true
			older = append(older, m)
		}
	}
	ch.msgs = append(older, ch.msgs...)
}

// mergeNew adds the messages of page the chat has not, which comes newest
// first, and keeps them in the order their times give.
func (ch *chatState) mergeNew(page []netease.Message) {
	seen := make(map[msgKey]bool, len(ch.msgs)+len(page))
	for _, m := range ch.msgs {
		seen[keyOf(m)] = true
	}
	for i := len(page) - 1; i >= 0; i-- {
		if m := page[i]; !seen[keyOf(m)] {
			seen[keyOf(m)] = true
			ch.msgs = append(ch.msgs, m)
		}
	}
	slices.SortFunc(ch.msgs, func(x, y netease.Message) int { return cmp.Compare(x.Time, y.Time) })
}

// inferPeer names the chat's other end from a message it sent, when the
// conversation that would name it is not loaded.
func (ch *chatState) inferPeer(uid int64) {
	if ch.peer.Nickname != "" {
		return
	}
	for _, m := range ch.msgs {
		if m.From.UserID == uid {
			ch.peer = m.From
			return
		}
	}
}

// refreshChat asks for the chat's latest page and merges what is new, as
// the poll does.
func (a *app) refreshChat(ch *chatState, uid int64) {
	if ch.refreshing || ch.loading {
		return
	}
	ch.refreshing = true
	fetch := privateHistory
	go func() {
		msgs, err := fetch(uid, 0, chatPageSize)
		a.post(func() {
			ch.refreshing = false
			if err == nil {
				ch.loaded = true
				if len(msgs) >= chatPageSize {
					ch.more = true
				}
				ch.mergeNew(msgs)
				ch.inferPeer(uid)
			}
		})
	}()
}

// pollChat sets the chat's next poll, which refreshes it while its page
// shows and stops as it closes or the listener signs out.
func (a *app) pollChat(ch *chatState, uid int64) {
	if ch.polling {
		return
	}
	ch.polling = true
	time.AfterFunc(chatPoll, func() {
		a.post(func() {
			ch.polling = false
			if a.route.page != pageChat || a.route.id != uid || a.profile == nil {
				return
			}
			a.refreshChat(ch, uid)
			a.pollChat(ch, uid)
		})
	})
}

// sendChat sends the chat's draft to its other end.
func (a *app) sendChat(ch *chatState, uid int64) {
	if ch.sending || strings.TrimSpace(ch.draft) == "" {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	text := strings.TrimSpace(ch.draft)
	ch.sending, ch.sendErr = true, ""
	send := sendPrivate
	go func() {
		m, err := send(uid, text)
		a.post(func() {
			ch.sending = false
			if err != nil {
				ch.sendErr = "发送失败，请稍后重试"
				return
			}
			ch.draft = ""
			if m != nil {
				if m.From.UserID == 0 && a.profile != nil {
					m.From.UserID = a.profile.UserID
				}
				ch.mergeNew([]netease.Message{*m})
			}
		})
	}()
}

// messagesPage lists the listener's private-message threads.
func (a *app) messagesPage(c *ui.Context) {
	if a.profile == nil {
		a.centered(c, "登录后查看私信", a.openLogin)
		return
	}
	ms := &a.msgs
	top, bottom := a.insets(c)
	if len(ms.convs) == 0 && (ms.loading || !ms.loaded && ms.err == "") {
		s := a.pageShimmer(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			a.startMark(c)
			a.heading(c, "私信", "网易云音乐站内私信")
			for i := range 8 {
				ui.Row(c.Key(i)).Height(64).Gap(12).AlignItems(ui.Center).Children(func() {
					s.bone(c, 20).Size(40, 40)
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
						s.text(c, 14, 1.3).WidthPercent(boneWidth(i) * 0.5)
						s.text(c, 13, 1.3).WidthPercent(boneWidth(i + 4))
					})
				})
			}
		})
		return
	}
	if len(ms.convs) == 0 && ms.err != "" {
		a.centered(c, ms.err, a.loadConversations)
		return
	}
	ui.Scroll(c.Key("messages")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+32, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, "私信", "网易云音乐站内私信")
		if len(ms.convs) == 0 {
			a.empty(c, "还没有私信")
			return
		}
		now := c.Now()
		for i := range ms.convs {
			a.conversationRow(c.Key(ms.convs[i].Peer.UserID), &ms.convs[i], now)
		}
		a.pageFoot(c, &ms.paged, a.loadConversations)
	})
}

// conversationRow is a thread of the list: the other end's avatar and
// name, what the last message said and when, and a pill counting the
// unread, which opening the thread reads.
func (a *app) conversationRow(c *ui.Context, cv *netease.Conversation, now time.Time) {
	p := a.pal
	b := ui.ButtonBase(c).Height(64).PaddingX(12).Gap(12).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(cv.Peer.Nickname)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		a.avatar(c, cv.Peer.AvatarURL, 40, p.press)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, cv.Peer.Nickname).FontSize(14).FontWeight(600).SingleLine()
			ui.Text(c, cv.LastMsg).FontSize(12.5).TextColor(p.muted).SingleLine()
		})
		ui.Column(c).AlignItems(ui.End).Gap(6).Children(func() {
			if cv.Time > 0 {
				ui.Text(c, commentTime(time.UnixMilli(cv.Time), now)).FontSize(11.5).TextColor(p.faint).SingleLine()
			}
			if cv.Unread > 0 {
				n := fmt.Sprintf("%d", min(cv.Unread, 99))
				if cv.Unread > 99 {
					n += "+"
				}
				ui.Box(c).Height(17).MinWidth(17).PaddingX(5).Radius(999).Center().Background(p.accent).Children(func() {
					ui.Text(c, n).FontSize(10.5).FontWeight(700).TextColor(p.onAccent).SingleLine()
				})
			}
		})
	})
	if b.Clicked() {
		a.chatFor(cv.Peer.UserID, cv.Peer)
		a.open(route{page: pageChat, id: cv.Peer.UserID})
		cv.Unread = 0
	}
}

// chatPage is a private-message thread: the other end at its head, the
// messages as bubbles, older loading above as they scroll, and a field
// at its foot to write in.
func (a *app) chatPage(c *ui.Context) {
	uid := a.route.id
	if a.profile == nil {
		a.centered(c, "登录后查看私信", a.openLogin)
		return
	}
	ch := a.chatFor(uid, a.peerOf(uid))
	a.pollChat(ch, uid)
	p := a.pal
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top, 0, 0, 0).Children(func() {
		head := ui.ButtonBase(c.Key("head")).Padding(pageGap, pagePad, 12, pagePad).Gap(12).Radius(8).AlignItems(ui.Center).FocusRing(false)
		name := ch.peer.Nickname
		if name == "" {
			name = "私信"
		}
		if ch.peer.Nickname != "" {
			head.Label("查看 " + name + " 的主页")
		} else {
			head.Label(name)
		}
		if head.Hovered() {
			head.Background(p.hover)
		}
		head.Children(func() {
			a.avatar(c, ch.peer.AvatarURL, 36, p.press)
			ui.Text(c, name).FontSize(17).FontWeight(700).SingleLine()
		})
		if head.Clicked() && ch.peer.UserID != 0 {
			a.open(route{page: pageUser, id: uid})
		}
		ui.Box(c).Height(0.5).Margin(0, pagePad).Background(p.hairline)
		switch {
		case !ch.loaded && ch.err == "":
			ui.Column(c).Grow(1).Padding(12, pagePad, 0, pagePad).Gap(4).Children(func() {
				s := a.pageShimmer(c)
				for i := range 6 {
					row := ui.Row(c.Key(i)).Padding(6, 0).Gap(8)
					if i%2 == 0 {
						row.Justify(ui.End)
					}
					row.Children(func() {
						if i%2 != 0 {
							s.bone(c, 15).Size(30, 30)
						}
						s.bone(c, 14).WidthPercent(boneWidth(i) * 0.5).Height(30)
					})
				}
			})
		case ch.err != "" && len(ch.msgs) == 0:
			a.centered(c, ch.err, func() { a.loadChat(ch, uid, false) })
		default:
			now := c.Now()
			ui.List(c.Key("chat"), &ch.list, len(ch.msgs)+1, func(i int) {
				if i == 0 {
					a.chatHead(c, ch, uid)
					return
				}
				a.messageRow(c, ch, i-1, now)
			}).Grow(1).PaddingY(6).ScrollbarInsets(0, 0, 8, 0)
		}
		a.chatEditor(c, ch, uid, bottom)
	})
}

// peerOf finds what is known of the other end of the chat with uid: the
// conversation of it, or the messages', or only its ID.
func (a *app) peerOf(uid int64) netease.MsgUser {
	for i := range a.msgs.convs {
		if cv := &a.msgs.convs[i]; cv.Peer.UserID == uid {
			return cv.Peer
		}
	}
	return netease.MsgUser{UserID: uid}
}

// chatHead is the row over a chat's messages: a way to load the next
// older page while there is one, or why it did not load.
func (a *app) chatHead(c *ui.Context, ch *chatState, uid int64) {
	p := a.pal
	ui.Row(c.Key("chat-head")).Height(40).Center().Children(func() {
		switch {
		case ch.loading && len(ch.msgs) > 0:
			ui.Text(c, "正在加载…").FontSize(12).TextColor(p.muted)
		case ch.err != "" && len(ch.msgs) > 0:
			ui.Text(c, ch.err).FontSize(12).TextColor(p.muted).Margin(0, 10, 0, 0)
			if ui.Button(c.Key("retry"), "重试").Clicked() {
				a.loadChat(ch, uid, true)
			}
		case ch.more:
			if ui.Button(c.Key("more"), "加载更早的消息").Clicked() {
				a.loadChat(ch, uid, true)
			}
		case len(ch.msgs) == 0:
			ui.Text(c, "打个招呼，开始聊天吧").FontSize(12.5).TextColor(p.muted)
		}
	})
}

// messageRow is one message of a chat, a bubble to the right of the
// listener's own and to the left of the other end's, its time over it
// once a pause of five minutes fell between it and the one before.
func (a *app) messageRow(c *ui.Context, ch *chatState, i int, now time.Time) {
	p := a.pal
	m := &ch.msgs[i]
	mine := a.profile != nil && m.From.UserID == a.profile.UserID
	ui.Column(c).PaddingX(pagePad).Children(func() {
		if i == 0 || m.Time-ch.msgs[i-1].Time > chatTimeGap {
			ui.Text(c, commentTime(time.UnixMilli(m.Time), now)).FontSize(11).TextColor(p.faint).AlignSelf(ui.Center).Padding(10, 0, 4, 0)
		}
		row := ui.Row(c).Padding(3, 0).Gap(8).AlignItems(ui.Start)
		if mine {
			row.Justify(ui.End)
		}
		row.Children(func() {
			if !mine {
				a.avatar(c, m.From.AvatarURL, 30, p.press)
			}
			bubble := ui.Box(c).Padding(8, 12).Radius(14).MaxWidthPercent(68)
			if mine {
				bubble.Background(p.accent).TextColor(p.onAccent)
			} else {
				bubble.Background(p.hover)
			}
			bubble.Children(func() {
				ui.Text(c, m.Text).FontSize(14).LineHeight(1.5)
			})
		})
	})
}

// chatEditor is the field at a chat's foot, which Enter or its button
// sends.
func (a *app) chatEditor(c *ui.Context, ch *chatState, uid int64, bottom float32) {
	p := a.pal
	ui.Row(c).Padding(8, pagePad, bottom+8, pagePad).Gap(10).AlignItems(ui.Center).Children(func() {
		field := ui.Row(c.Key("field")).Grow(1).Radius(999).AlignItems(ui.Center)
		if c.Focused(a.chatBox) {
			field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
		} else {
			field.Background(p.hover).Border(1, p.hairline)
		}
		field.Children(func() {
			in := ui.TextInputBase(c.Key("draft"), &ch.draft).Bind(&a.chatBox).ReadOnly(ch.sending).
				Placeholder("发私信…").Label("私信内容").FontSize(14).Grow(1).Padding(8, 14)
			if in.Submitted() {
				a.sendChat(ch, uid)
			}
		})
		if ch.sendErr != "" {
			ui.Text(c, ch.sendErr).FontSize(12).TextColor(p.muted).SingleLine()
		}
		b := iconButton(c.Key("send"), iconSend, 16, p.accent, "发送")
		if ch.sending || strings.TrimSpace(ch.draft) == "" {
			b.Disabled(true).Opacity(0.35)
		}
		if b.Clicked() {
			a.sendChat(ch, uid)
		}
	})
}
