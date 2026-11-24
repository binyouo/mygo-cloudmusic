package netease

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Private messages between the listener and other users, over the weapi
// as the website's message page sends them.

// MsgUser is one end of a private-message conversation.
type MsgUser struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

// Conversation is a private-message thread the listener has: who it is
// with, what its last message said and when, and how many are unread.
type Conversation struct {
	Peer    MsgUser
	LastMsg string
	Time    int64 // Unix milliseconds
	Unread  int
}

// Message is one private message: who sent it, what it says, and when.
type Message struct {
	From MsgUser
	Text string
	Time int64 `json:"time"` // Unix milliseconds
}

// wireMsg is a message as it comes, its text inside its msg, a JSON
// string of its own.
type wireMsg struct {
	From MsgUser `json:"fromUser"`
	To   MsgUser `json:"toUser"`
	Msg  string  `json:"msg"`
	Time int64   `json:"time"`
}

func (w *wireMsg) message() Message {
	return Message{From: w.From, Text: msgText(w.Msg), Time: w.Time}
}

// PrivateConversations returns the listener's private-message threads,
// the most recent first: limit of them from offset. mine, the listener's
// ID, picks each thread's other end out of a message's two.
func PrivateConversations(mine int64, offset, limit int) ([]Conversation, error) {
	var res struct {
		apiError
		Msgs []struct {
			From        MsgUser `json:"fromUser"`
			To          MsgUser `json:"toUser"`
			LastMsg     string  `json:"lastMsg"`
			LastMsgTime int64   `json:"lastMsgTime"`
			Unread      int     `json:"newMsgCount"`
		} `json:"msgs"`
	}
	data := map[string]any{"offset": offset, "limit": limit, "total": true}
	if err := weapiPost("/api/msg/private/users", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: private users: code %d %s", res.Code, res.Message)
	}
	convs := make([]Conversation, 0, len(res.Msgs))
	for _, m := range res.Msgs {
		peer := m.From
		if peer.UserID == mine {
			peer = m.To
		}
		convs = append(convs, Conversation{
			Peer: peer, LastMsg: msgText(m.LastMsg), Time: m.LastMsgTime, Unread: m.Unread,
		})
	}
	return convs, nil
}

// PrivateHistory returns limit of the messages between the listener and
// uid, the newest first, those sent before beforetime when nonzero.
func PrivateHistory(uid, beforetime int64, limit int) ([]Message, error) {
	var res struct {
		apiError
		Msgs []wireMsg `json:"msgs"`
	}
	data := map[string]any{
		"userId": strconv.FormatInt(uid, 10),
		"limit":  limit,
		"time":   beforetime,
		"total":  true,
	}
	if err := weapiPost("/api/msg/private/history", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: private history: code %d %s", res.Code, res.Message)
	}
	msgs := make([]Message, 0, len(res.Msgs))
	for i := range res.Msgs {
		msgs = append(msgs, res.Msgs[i].message())
	}
	return msgs, nil
}

// SendPrivate sends text to uid and returns it as NetEase recorded it,
// or, it saying none, as sent.
func SendPrivate(uid int64, text string) (*Message, error) {
	var res struct {
		apiError
		NewMsgs []wireMsg `json:"newMsgs"`
	}
	data := map[string]any{"type": "text", "msg": text, "userIds": "[" + strconv.FormatInt(uid, 10) + "]"}
	if err := weapiPost("/api/msg/private/send", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		msg := res.Message
		if msg == "" {
			msg = res.Msg
		}
		return nil, fmt.Errorf("netease: private send: code %d %s", res.Code, msg)
	}
	if len(res.NewMsgs) == 0 {
		return &Message{Text: text, Time: time.Now().UnixMilli()}, nil
	}
	m := res.NewMsgs[0].message()
	return &m, nil
}

// msgText is what a message says: its msg, a JSON string of its own for
// every kind, holds the text of a text message, or what a shared song,
// playlist, album, program, MV or event is.
func msgText(raw string) string {
	var inner map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &inner) != nil {
		return raw
	}
	var text string
	if err := json.Unmarshal(inner["msg"], &text); err == nil && text != "" {
		return text
	}
	for k, kind := range map[string]string{
		"song": "歌曲", "playlist": "歌单", "album": "专辑",
		"program": "节目", "mv": "MV", "event": "动态",
	} {
		v, ok := inner[k]
		if !ok {
			continue
		}
		var obj struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(v, &obj) == nil && obj.Name != "" {
			return "分享了" + kind + "《" + obj.Name + "》"
		}
		return "分享了" + kind
	}
	return "…"
}
