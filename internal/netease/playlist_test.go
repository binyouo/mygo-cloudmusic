package netease

import (
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"strings"
	"testing"
)

type playlistTransport func(*http.Request) (*http.Response, error)

func (f playlistTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The initial batch follows track IDs even if details are reordered or omitted; failures must not become a complete-looking playlist.
func TestPlaylistInitialBatchOrderAndFailure(t *testing.T) {
	oldClient, oldTransport := httpClient, http.DefaultTransport
	defer func() { httpClient, http.DefaultTransport = oldClient, oldTransport }()
	jar, _ := cookiejar.New(nil)
	for _, fail := range []bool{false, true} {
		var requested []int64
		transport := playlistTransport(func(r *http.Request) (*http.Response, error) {
			var body []byte
			switch r.URL.Path {
			case "/api/v6/playlist/detail":
				p := Playlist{ID: 42, Tracks: []Track{{ID: 2, Name: "二"}, {ID: 1, Name: "一"}}}
				for i := range 1005 {
					p.TrackIDs = append(p.TrackIDs, struct {
						ID int64 `json:"id"`
					}{int64(i + 1)})
				}
				body, _ = json.Marshal(map[string]any{"code": 200, "playlist": p})
			case "/eapi/v3/song/detail":
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				sealed, err := hex.DecodeString(r.Form.Get("params"))
				if err != nil || len(sealed)%aes.BlockSize != 0 {
					t.Fatal("invalid encrypted request")
				}
				block, _ := aes.NewCipher([]byte(eapiKey))
				plain := make([]byte, len(sealed))
				for i := 0; i < len(sealed); i += aes.BlockSize {
					block.Decrypt(plain[i:i+aes.BlockSize], sealed[i:i+aes.BlockSize])
				}
				parts := strings.Split(string(plain), "-36cd479b6b5-")
				if len(parts) != 3 {
					t.Fatal("invalid signed request")
				}
				var data struct {
					C string `json:"c"`
				}
				if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
					t.Fatal(err)
				}
				var ids []struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal([]byte(data.C), &ids); err != nil {
					t.Fatal(err)
				}
				if len(ids) > 400 {
					t.Fatal("unbounded details request")
				}
				var songs []Track
				for _, id := range ids {
					requested = append(requested, id.ID)
					if id.ID != 7 {
						songs = append(songs, Track{ID: id.ID, Name: fmt.Sprint(id.ID)})
					}
				}
				slices.Reverse(songs)
				if fail && ids[0].ID >= 3 {
					body = []byte(`{"code":500}`)
				} else {
					body, _ = json.Marshal(map[string]any{"code": 200, "songs": songs})
				}
			default:
				t.Fatalf("unexpected route %s", r.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		})
		httpClient, http.DefaultTransport = &http.Client{Jar: jar, Transport: transport}, transport
		p, err := PlaylistDetail(42)
		if fail {
			if err == nil || p != nil {
				t.Fatal("details failure silently truncated the playlist")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if p.TrackOffset != 1000 || len(p.TrackIDs) != 1005 || len(p.Tracks) != 999 {
			t.Fatalf("offset %d, IDs %d, tracks %d", p.TrackOffset, len(p.TrackIDs), len(p.Tracks))
		}
		if slices.Contains(requested, int64(1001)) {
			t.Fatal("initial load fetched past its batch")
		}
		for i, tr := range p.Tracks {
			want := int64(i + 1)
			if want >= 7 {
				want++
			}
			if tr.ID != want {
				t.Fatalf("row %d: ID %d, want %d", i, tr.ID, want)
			}
		}
	}
}
