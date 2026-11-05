package netease

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// API rejection must not look like an unavailable song and make playback skip the queue.
func TestSongURLRejectsAPIError(t *testing.T) {
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	for _, body := range []string{
		`{"code":-460,"message":"检测到您的网络环境存在风险，请稍后再试"}`,
		`{"code":301,"message":"login required"}`,
		`{"code":429,"message":"too many requests"}`,
		`{"code":500,"msg":"service unavailable"}`,
	} {
		http.DefaultTransport = weapiTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		_, err := SongURL(1, "hires")
		if err == nil || errors.Is(err, ErrUnavailable) {
			t.Fatalf("API rejection became a missing song: %s: %v", body, err)
		}
	}
}

// Successful replies still distinguish a full stream from a missing source or a paid preview.
func TestSongURLStreamAvailability(t *testing.T) {
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	for _, tc := range []struct {
		body    string
		missing bool
	}{
		{`{"code":200,"data":[{"url":"https://example.test/song.flac","type":"flac","level":"hires","size":1234}]}`, false},
		{`{"code":200,"data":[{"url":null}]}`, true},
		{`{"code":200,"data":[{"url":"https://example.test/preview.mp3","freeTrialInfo":{"start":0,"end":30}}]}`, true},
	} {
		http.DefaultTransport = weapiTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})
		st, err := SongURL(1, "hires")
		if tc.missing {
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("missing source: %v", err)
			}
		} else if err != nil || st == nil || st.Level != "hires" || st.URL != "https://example.test/song.flac" {
			t.Fatalf("lost full stream: %+v, %v", st, err)
		}
	}
}
