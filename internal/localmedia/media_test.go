package localmedia

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

func taggedMP3(t *testing.T, dir string) string {
	t.Helper()
	audio, err := os.ReadFile("../player/testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(audio, []byte("ID3")) {
		size := int(audio[6])<<21 | int(audio[7])<<14 | int(audio[8])<<7 | int(audio[9])
		audio = audio[10+size:]
	}
	var picture bytes.Buffer
	cover := image.NewRGBA(image.Rect(0, 0, 4, 4))
	cover.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&picture, cover); err != nil {
		t.Fatal(err)
	}
	var frames bytes.Buffer
	frame := func(name string, data []byte) {
		frames.WriteString(name)
		binary.Write(&frames, binary.BigEndian, uint32(len(data)))
		frames.Write([]byte{0, 0})
		frames.Write(data)
	}
	frame("TIT2", append([]byte{3}, []byte("测试歌曲")...))
	frame("TPE1", append([]byte{3}, []byte("测试歌手")...))
	frame("TALB", append([]byte{3}, []byte("测试专辑")...))
	frame("USLT", append([]byte{3, 'e', 'n', 'g', 0}, []byte("[00:02]内嵌歌词")...))
	frame("APIC", append([]byte{3, 'i', 'm', 'a', 'g', 'e', '/', 'p', 'n', 'g', 0, 0, 0}, picture.Bytes()...))
	size := frames.Len()
	data := []byte{'I', 'D', '3', 3, 0, 0, byte(size >> 21 & 127), byte(size >> 14 & 127), byte(size >> 7 & 127), byte(size & 127)}
	data = append(data, frames.Bytes()...)
	data = append(data, audio...)
	path := filepath.Join(dir, "中文 空格.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// An MP3's APIC picture of type Other must display alongside its embedded lyrics without changing the audio file.
func TestReadEmbeddedCoverAndLyrics(t *testing.T) {
	dir := t.TempDir()
	path := taggedMP3(t, dir)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	track, err := Read(path, filepath.Join(dir, "封面 空格"))
	if err != nil {
		t.Fatal(err)
	}
	if track.ID >= 0 || track.Name != "测试歌曲" || track.ArtistNames() != "测试歌手" || track.Album.Name != "测试专辑" || track.Duration <= 0 {
		t.Fatalf("metadata missing: %+v", track)
	}
	if track.LocalLyrics != "[00:02]内嵌歌词" {
		t.Fatalf("missing lyrics: %q", track.LocalLyrics)
	}
	data, err := netease.Fetch(netease.Sized(track.Album.PicURL, 600))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("cover cannot be decoded: %v", err)
	}
	again, err := Read(path, filepath.Join(dir, "封面 空格"))
	if err != nil || again.ID != track.ID {
		t.Fatalf("file identity changed: %+v, %v", again, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("import modified the audio file")
	}
}

// A UTF-16 sidecar overrides embedded lyrics and keeps its timestamps for synchronized playback.
func TestSidecarLyricsOverrideEmbedded(t *testing.T) {
	dir := t.TempDir()
	path := taggedMP3(t, dir)
	data, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte("[00:03.500]外置歌词"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path[:len(path)-len(filepath.Ext(path))]+".lrc", data, 0o600); err != nil {
		t.Fatal(err)
	}
	track, err := Read(path, filepath.Join(dir, "art"))
	if err != nil {
		t.Fatal(err)
	}
	lines := lyrics.Build(track.LocalLyrics, "", "")
	if len(lines) != 1 || lines[0].At != 3500*time.Millisecond || lines[0].Text != "外置歌词" {
		t.Fatalf("sidecar timing lost: %+v", lines)
	}
}

// A FLAC keeps its sample-derived duration even when it has no embedded artwork or lyrics.
func TestReadFLAC(t *testing.T) {
	track, err := Read("../player/testdata/seek.flac", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if track.Duration <= 0 || track.LocalPath == "" || track.ID >= 0 {
		t.Fatalf("missing FLAC metadata: %+v", track)
	}
}

// GBK and mixed-case LRC sidecars decode as Chinese lyrics rather than unreadable text.
func TestReadGBKLyrics(t *testing.T) {
	dir := t.TempDir()
	text := "[00:01.50]中文歌词\r[00:02.50]第二句"
	data, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "歌曲.LrC"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded, err := SidecarLyrics(filepath.Join(dir, "歌曲.mp3"))
	if err != nil || decoded != text {
		t.Fatalf("GBK sidecar failed: %q, %v", decoded, err)
	}
	lines := lyrics.Build(decoded, "", "")
	if len(lines) != 2 || lines[0].Text != "中文歌词" || lines[1].At != 2500*time.Millisecond {
		t.Fatalf("decoded lyric timing lost: %+v", lines)
	}
}

// Optional real-file verification extracts the listener's cover and LRC into a temporary directory and preserves the source hash.
func TestReadSample(t *testing.T) {
	path := os.Getenv("LOCAL_AUDIO_SAMPLE")
	if path == "" {
		t.Skip("set LOCAL_AUDIO_SAMPLE to inspect an audio file")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	track, err := Read(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cover, err := netease.Fetch(netease.Sized(track.Album.PicURL, 600))
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	lines := lyrics.Build(track.LocalLyrics, "", "")
	if len(lines) == 0 || track.Duration <= 0 {
		t.Fatalf("missing timed lyrics or duration: %d lines, %d ms", len(lines), track.Duration)
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source audio changed")
	}
	t.Logf("%s / %s / %s: cover=%v, lyrics=%d lines, duration=%d ms, source hash unchanged", track.Name, track.ArtistNames(), track.Album.Name, decoded.Bounds().Size(), len(lines), track.Duration)
}
