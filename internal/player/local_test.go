package player

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Importing matching local metadata repairs a cloud upload without changing its stream or accepting an older lyric response.
func TestCloudSongUsesImportedMetadata(t *testing.T) {
	oldLyrics, oldArt := fetchLyrics, fetchArtwork
	defer func() { fetchLyrics, fetchArtwork = oldLyrics, oldArt }()
	cloudRequests := make(chan int64, 4)
	fetchLyrics = func(id int64) (string, string, string, error) {
		cloudRequests <- id
		return "[00:01]旧的云端歌词", "", "", nil
	}
	artRequests := make(chan string, 4)
	fetchArtwork = func(url string) ([]byte, error) {
		artRequests <- url
		return nil, fmt.Errorf("no native artwork in test")
	}
	posted := make(chan func(), 4)
	p := New(func(fn func()) { posted <- fn })
	cloud := netease.Track{ID: 42, Name: "鸟戏outro", Artists: []netease.Artist{{Name: "LSGCsikoriot"}}, Duration: 132024}
	p.Cue([]netease.Track{cloud})
	p.Hold(20 * time.Second)
	p.loadMetadata(cloud, p.gen)
	oldResponse := <-posted
	<-cloudRequests
	path := filepath.Join(t.TempDir(), "鸟戏outro.lrc")
	if err := os.WriteFile(path, []byte("[00:02]本地同步歌词"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := cloud
	local.ID, local.LocalPath = -7, "/music/鸟戏outro.mp3"
	local.LocalLyricsPath, local.LocalLyrics = path, "[00:01]旧快照"
	local.Album.PicURL = "file:///cover.jpg"
	p.SetLocalLibrary([]netease.Track{local})
	select {
	case apply := <-posted:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("metadata job did not finish")
	}
	oldResponse()
	lines, loaded := p.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "本地同步歌词" {
		t.Fatalf("import lost to cloud metadata: %+v", lines)
	}
	if p.Current().ID != 42 || p.Current().LocalPath != "" || p.Current().Album.PicURL != local.Album.PicURL || p.Position() != 20*time.Second || !p.Playing() {
		t.Fatalf("cloud playback was changed: %+v", p.Current())
	}
	if url := <-artRequests; url != local.Album.PicURL {
		t.Fatalf("wrong artwork: %s", url)
	}
	select {
	case id := <-cloudRequests:
		t.Fatalf("fetched cloud lyrics instead of imported lyrics for %d", id)
	default:
	}
}

// A local MP3 opens without a network cache, supports seeking, and stays intact when the decoder closes.
func TestLocalAudioOpenAndSeek(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "local.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	p := New(func(func()) {})
	info, decoded, err := p.open(netease.Track{ID: -7, LocalPath: path, Duration: 3000}, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != "mp3" || info.Size != int64(len(data)) {
		t.Fatalf("wrong local stream: %+v", info)
	}
	buffer := make([]float32, 4096)
	if count, err := decoded.Read(buffer); count == 0 || err != nil {
		t.Fatalf("cannot decode local audio: %d, %v", count, err)
	}
	if err := decoded.SetPosition(int64(decoded.SampleRate())); err != nil {
		t.Fatal(err)
	}
	if count, err := decoded.Read(buffer); count == 0 || err != nil {
		t.Fatalf("cannot seek local audio: %d, %v", count, err)
	}
	if err := decoded.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("closing local playback changed or deleted its source")
	}
}

// Local lyrics bypass NetEase and obsolete metadata jobs cannot replace the next song's lyrics.
func TestLocalMetadataDoesNotRequestCloudLyrics(t *testing.T) {
	original := fetchLyrics
	defer func() { fetchLyrics = original }()
	cloud := make(chan int64, 2)
	fetchLyrics = func(id int64) (string, string, string, error) {
		cloud <- id
		return "", "", "", nil
	}
	posted := make(chan func(), 4)
	p := New(func(fn func()) { posted <- fn })
	track := netease.Track{ID: -7, LocalPath: "local.mp3", LocalLyrics: "[00:01]本地歌词"}
	p.Cue([]netease.Track{track})
	apply := func() {
		t.Helper()
		select {
		case fn := <-posted:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("metadata job did not finish")
		}
	}
	p.loadMetadata(track, p.gen)
	apply()
	lines, loaded := p.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "本地歌词" {
		t.Fatalf("local lyrics missing: %+v", lines)
	}
	p.loadMetadata(netease.Track{ID: -8, LocalPath: "old.mp3", LocalLyrics: "[00:01]旧歌词"}, p.gen)
	p.gen++
	apply()
	lines, _ = p.Lyrics()
	if lines[0].Text != "本地歌词" {
		t.Fatal("obsolete metadata replaced the current lyrics")
	}
	select {
	case id := <-cloud:
		t.Fatalf("requested cloud lyrics for local track %d", id)
	default:
	}
}

// A sidecar added after the audio was imported is read on playback instead of using an empty catalog snapshot.
func TestLocalMetadataRefreshesSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "歌曲.mp3")
	if err := os.WriteFile(filepath.Join(dir, "歌曲.lrc"), []byte("[00:03]新加的歌词"), 0o600); err != nil {
		t.Fatal(err)
	}
	posted := make(chan func(), 4)
	p := New(func(fn func()) { posted <- fn })
	track := netease.Track{ID: -7, LocalPath: path}
	p.Cue([]netease.Track{track})
	p.Hold(10 * time.Second)
	p.loadMetadata(track, p.gen)
	select {
	case apply := <-posted:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("metadata job did not finish")
	}
	lines, loaded := p.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "新加的歌词" || p.track.LocalLyrics == "" || p.Position() != 10*time.Second {
		t.Fatalf("late sidecar was not loaded: %+v", lines)
	}
}

// Importing a lyric during playback wins over an older metadata job without restarting or seeking the song.
func TestLocalLyricsImportWinsOverPendingMetadata(t *testing.T) {
	posted := make(chan func(), 4)
	p := New(func(fn func()) { posted <- fn })
	track := netease.Track{ID: -7, LocalPath: filepath.Join(t.TempDir(), "歌曲.mp3"), LocalLyrics: "[00:01]旧歌词"}
	p.Cue([]netease.Track{track})
	p.Hold(20 * time.Second)
	p.loadMetadata(track, p.gen)
	var apply func()
	select {
	case apply = <-posted:
	case <-time.After(5 * time.Second):
		t.Fatal("metadata job did not finish")
	}
	p.UpdateLocalLyrics(track.LocalPath, "/custom.lrc", "[00:02]新歌词")
	apply()
	lines, loaded := p.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "新歌词" || p.Position() != 20*time.Second || !p.Playing() || p.Queue()[0].LocalLyricsPath != "/custom.lrc" {
		t.Fatalf("lyric import was overwritten or interrupted playback: %+v", lines)
	}
}

// A song prepared before an LRC import uses the queue's newer lyric association when it becomes current.
func TestLocalPreparedTrackUsesUpdatedLyrics(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "独立歌词.lrc")
	if err := os.WriteFile(source, []byte("[00:02]预加载之后导入的歌词"), 0o600); err != nil {
		t.Fatal(err)
	}
	posted := make(chan func(), 4)
	p := New(func(fn func()) { posted <- fn })
	prepared := netease.Track{ID: -7, LocalPath: filepath.Join(dir, "歌曲.mp3")}
	p.Cue([]netease.Track{prepared})
	p.UpdateLocalLyrics(prepared.LocalPath, source, "[00:02]预加载之后导入的歌词")
	p.track = &prepared
	p.loadMetadata(prepared, p.gen)
	select {
	case apply := <-posted:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("metadata job did not finish")
	}
	lines, loaded := p.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "预加载之后导入的歌词" || p.track.LocalLyricsPath != source {
		t.Fatalf("prepared audio lost its updated lyric association: %+v", lines)
	}
}
