package localmedia

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Old MP3 tags written as GBK or UTF-8 in Latin-1 fields recover their text without corrupting genuine Western tags.
func TestDecodeLegacy(t *testing.T) {
	latin1 := func(b string) string {
		r := make([]rune, len(b))
		for i := range len(b) {
			r[i] = rune(b[i])
		}
		return string(r)
	}
	for _, c := range []struct{ in, want string }{
		{latin1("\xb4\xf8\xc0\xe1\xb5\xc4\xd3\xe3"), "带泪的鱼"},
		{"\xd5\xd4\xc0\xd7", "赵雷"},
		{latin1("\xe8\xb5\xb5\xe9\x9b\xb7"), "赵雷"},
		{latin1("Mot\xf6rhead"), "Motörhead"},
		{"Caf\xe9", "Café"},
		{"Hebe 田馥甄", "Hebe 田馥甄"},
	} {
		if got := decodeLegacy(c.in); got != c.want {
			t.Errorf("decode %q: got %q, want %q", c.in, got, c.want)
		}
	}
}

// Reading legacy MP3 files repairs their actual title, artist, album and embedded lyrics rather than only the decoder helper.
func TestReadLegacyMP3Tags(t *testing.T) {
	audio, err := os.ReadFile("../player/testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(audio, []byte("ID3")) {
		n := int(audio[6])<<21 | int(audio[7])<<14 | int(audio[8])<<7 | int(audio[9])
		audio = audio[10+n:]
	}
	gbk := func(s string) []byte {
		b, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			data := append([]byte(nil), audio...)
			if version == "v1" {
				tag := make([]byte, 128)
				copy(tag, "TAG")
				copy(tag[3:33], gbk("中文标题"))
				copy(tag[33:63], gbk("中文歌手"))
				copy(tag[63:93], gbk("中文专辑"))
				data = append(data, tag...)
			} else {
				var frames bytes.Buffer
				frame := func(id string, b []byte) {
					frames.WriteString(id)
					binary.Write(&frames, binary.BigEndian, uint32(len(b)))
					frames.Write([]byte{0, 0})
					frames.Write(b)
				}
				frame("TIT2", append([]byte{0}, gbk("中文标题")...))
				frame("TPE1", append([]byte{0}, gbk("中文歌手")...))
				frame("TALB", append([]byte{0}, gbk("中文专辑")...))
				frame("USLT", append([]byte{0, 'z', 'h', 'o', 0}, gbk("[00:01]内嵌歌词")...))
				n := frames.Len()
				header := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 127), byte(n >> 14 & 127), byte(n >> 7 & 127), byte(n & 127)}
				data = append(append(header, frames.Bytes()...), data...)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "legacy.mp3")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			tr, err := Read(path, filepath.Join(dir, "art"))
			if err != nil {
				t.Fatal(err)
			}
			if tr.Name != "中文标题" || tr.ArtistNames() != "中文歌手" || tr.Album.Name != "中文专辑" {
				t.Fatalf("tags %+v", tr)
			}
			if version == "v2" && tr.LocalLyrics != "[00:01]内嵌歌词" {
				t.Fatalf("lyrics %q", tr.LocalLyrics)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, data) {
				t.Fatal("reading tags modified the source")
			}
		})
	}
}

// Ogg import reads the duration and sidecar lyrics so the imported song can seek and display synchronized lyrics.
func TestReadOgg(t *testing.T) {
	b, err := os.ReadFile("../player/testdata/seek.ogg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "歌曲.OGG")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "歌曲.lrc"), []byte("[00:01]歌词"), 0600)
	tr, err := Read(path, filepath.Join(dir, "art"))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if tr.LocalPath != canonical || tr.Duration < 4000 || tr.LocalLyrics != "[00:01]歌词" {
		t.Fatalf("imported %+v", tr)
	}
}
