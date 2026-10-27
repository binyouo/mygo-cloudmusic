// Package unblock finds an alternative stream for an unavailable song.
// The Bodian protocol is adapted from UnblockNeteaseMusic/server's
// src/provider/bodian.js (LGPL-3.0-only); see LICENSE.unblock.
package unblock

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/netease"
)

var client = &http.Client{Timeout: 12 * time.Second}
var deviceID = strconv.FormatUint(rand.Uint64N(100000000000), 10)

type candidate struct {
	ID       string `json:"MUSICRID"`
	Name     string `json:"SONGNAME"`
	Artist   string `json:"ARTIST"`
	Duration string `json:"DURATION"`
}

// match only accepts the same title, artists and duration. In particular,
// a cover of a popular song must not stand in for the original.
func match(t netease.Track, c candidate) bool {
	if t.Name == "" || len(t.Artists) == 0 || t.Duration <= 0 || !strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(c.Name)) {
		return false
	}
	artists := strings.Split(c.Artist, "&")
	if len(artists) != len(t.Artists) {
		return false
	}
	for i, a := range artists {
		if !strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(t.Artists[i].Name)) {
			return false
		}
	}
	seconds, err := strconv.ParseInt(c.Duration, 10, 64)
	return err == nil && seconds > 0 && abs(seconds*1000-t.Duration) < 5000 && strings.HasPrefix(c.ID, "MUSIC_")
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func getJSON(req *http.Request, out any) error {
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("source: %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}

// Song finds an MP3 from Bodian when NetEase has no full stream.
func Song(t netease.Track) (*netease.Stream, error) {
	if t.ProgramID != 0 || t.Name == "" || len(t.Artists) == 0 || t.Duration <= 0 {
		return nil, netease.ErrUnavailable
	}
	keyword := t.Name + " " + t.ArtistNames()
	search := "http://search.kuwo.cn/r.s?correct=1&vipver=1&stype=comprehensive&encoding=utf8&rformat=json&mobi=1&show_copyright_off=1&searchapi=6&all=" + url.QueryEscape(keyword)
	req, err := http.NewRequest(http.MethodGet, search, nil)
	if err != nil {
		return nil, err
	}
	var found struct {
		Content []struct {
			MusicPage struct {
				List []candidate `json:"abslist"`
			} `json:"musicpage"`
		} `json:"content"`
	}
	if err = getJSON(req, &found); err != nil {
		return nil, err
	}
	var id string
	for _, page := range found.Content {
		for _, c := range page.MusicPage.List {
			if match(t, c) {
				id = strings.TrimPrefix(c.ID, "MUSIC_")
				break
			}
		}
		if id != "" {
			break
		}
	}
	if id == "" {
		return nil, netease.ErrUnavailable
	}
	ad, err := http.NewRequest(http.MethodPost, "http://bd-api.kuwo.cn/api/service/advert/watch?uid=-1&token=&timestamp=1724306124436&sign=15a676d66285117ad714e8c8371691da", strings.NewReader(`{"type":5,"subType":5,"musicId":0,"adToken":""}`))
	if err != nil {
		return nil, err
	}
	bodianHeaders(ad)
	ad.Header.Set("qimei36", "1e9970cbcdc20a031dee9f37100017e1840e")
	ad.Header.Set("Content-Type", "application/json; charset=utf-8")
	res, err := client.Do(ad)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, netease.ErrUnavailable
	}
	// The source signs the alphanumeric query characters in sorted order.
	path := "/api/play/music/v2/audioUrl"
	query := "&br=320kmp3&musicId=" + url.QueryEscape(id) + "&timestamp=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	chars := make([]byte, 0, len(query))
	for _, c := range []byte(query) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			chars = append(chars, c)
		}
	}
	slices.Sort(chars)
	sign := md5.Sum([]byte("kuwotest" + string(chars) + path))
	req, err = http.NewRequest(http.MethodGet, "http://bd-api.kuwo.cn"+path+"?"+query+fmt.Sprintf("&sign=%x", sign), nil)
	if err != nil {
		return nil, err
	}
	bodianHeaders(req)
	req.Header.Set("X-Forwarded-For", "1.0.1.114")
	var response struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"audioUrl"`
		} `json:"data"`
	}
	if err = getJSON(req, &response); err != nil {
		return nil, err
	}
	u, err := url.Parse(response.Data.URL)
	if response.Code != 200 || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, netease.ErrUnavailable
	}
	return &netease.Stream{URL: u.String(), Type: "mp3", Level: "exhigh"}, nil
}

func bodianHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Dart/2.19 (dart:io)")
	req.Header.Set("plat", "ar")
	req.Header.Set("channel", "aliopen")
	req.Header.Set("devid", deviceID)
	req.Header.Set("ver", "3.9.0")
}
