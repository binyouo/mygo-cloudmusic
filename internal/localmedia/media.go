package localmedia

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dhowden/tag"
	gomp3 "github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"

	"netease-music-mygo/internal/netease"
)

func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac":
		return true
	}
	return false
}

func Read(path, artworkDir string) (netease.Track, error) {
	var track netease.Track
	if !Supported(path) {
		return track, fmt.Errorf("支持导入 MP3 和 FLAC")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return track, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return track, err
	}
	file, err := os.Open(path)
	if err != nil {
		return track, err
	}
	defer file.Close()
	key := sha256.Sum256([]byte(path))
	track.ID = -int64(binary.BigEndian.Uint64(key[:8])>>1) - 1
	track.LocalPath = path
	track.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	metadata, tagErr := tag.ReadFrom(file)
	if tagErr == nil {
		if metadata.Title() != "" {
			track.Name = metadata.Title()
		}
		track.Artists = []netease.Artist{{Name: metadata.Artist()}}
		track.Album.Name = metadata.Album()
		track.LocalLyrics = metadata.Lyrics()
		if picture := metadata.Picture(); picture != nil {
			track.Album.PicURL, err = keepArtwork(artworkDir, picture.Data)
			if err != nil {
				return track, err
			}
		}
	}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if text, readErr := SidecarLyrics(path); readErr == nil {
		track.LocalLyrics = text
	}
	if track.Album.PicURL == "" {
		for _, name := range []string{stem + ".jpg", stem + ".png", filepath.Join(filepath.Dir(path), "cover.jpg"), filepath.Join(filepath.Dir(path), "folder.jpg")} {
			if data, readErr := os.ReadFile(name); readErr == nil {
				track.Album.PicURL, err = keepArtwork(artworkDir, data)
				if err != nil {
					return track, err
				}
				break
			}
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return track, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		decoder, decodeErr := gomp3.NewDecoder(file)
		if decodeErr != nil {
			return track, decodeErr
		}
		track.Duration = decoder.Length() * 1000 / 4 / int64(decoder.SampleRate())
	case ".flac":
		stream, decodeErr := flac.New(file)
		if decodeErr != nil {
			return track, decodeErr
		}
		track.Duration = int64(stream.Info.NSamples) * 1000 / int64(stream.Info.SampleRate)
	}
	return track, nil
}

func keepArtwork(dir string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	key := sha256.Sum256(data)
	path := filepath.Join(dir, fmt.Sprintf("%x.cover", key))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String(), nil
}

func decodeLyrics(data []byte) string {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		decoded, err := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder().Bytes(data)
		if err == nil {
			return string(decoded)
		}
	}
	if !utf8.Valid(data) {
		decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
		if err == nil {
			return string(decoded)
		}
	}
	return strings.TrimPrefix(string(data), "\ufeff")
}

func ReadLyrics(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := decodeLyrics(data)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("歌词文件为空")
	}
	return text, nil
}

func SidecarLyrics(path string) (string, error) {
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	text, err := ReadLyrics(stem + ".lrc")
	if err == nil || !os.IsNotExist(err) {
		return text, err
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), filepath.Base(stem)+".lrc") {
			return ReadLyrics(filepath.Join(filepath.Dir(path), entry.Name()))
		}
	}
	return "", os.ErrNotExist
}
