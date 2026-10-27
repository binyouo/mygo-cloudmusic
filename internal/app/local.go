package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/localmedia"
	"netease-music-mygo/internal/netease"
)

func localLibraryPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-library.json"), nil
}

func (a *app) loadLocalLibrary() {
	path, err := localLibraryPath()
	if err == nil {
		var data []byte
		data, err = os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(data, &a.localTracks)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.localError = "本地音乐库读取失败：" + err.Error()
	}
	a.pl.SetLocalLibrary(a.localTracks)
}

func saveLocalLibrary(path string, tracks []netease.Track) error {
	data, err := json.Marshal(tracks)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path+".part", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".part", path)
}

func (a *app) chooseLocalAudio() {
	if a.localImporting {
		return
	}
	a.showMain()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "导入本地音乐", Multiple: true,
			Filters: []mygo.FileFilter{{Name: "音乐与歌词", Extensions: []string{"mp3", "flac", "lrc"}}},
		})
		a.post(func() {
			if err != nil {
				a.toast = "无法打开文件选择器：" + err.Error()
				return
			}
			if len(paths) > 0 {
				a.importLocalFiles(paths)
			}
		})
	}()
}

func mergeLocalTracks(existing, imported []netease.Track) []netease.Track {
	tracks := append([]netease.Track(nil), existing...)
	indices := make(map[int64]int, len(tracks))
	for index, track := range tracks {
		indices[track.ID] = index
	}
	for _, track := range imported {
		if index, found := indices[track.ID]; found {
			tracks[index] = track
		} else {
			indices[track.ID] = len(tracks)
			tracks = append(tracks, track)
		}
	}
	return tracks
}

type localImportResult struct {
	tracks        []netease.Track
	updated       map[int64]bool
	audio, lyrics int
	failures      []string
}

func readLocalImports(paths []string, existing []netease.Track, currentPath, artworkDir string) localImportResult {
	result := localImportResult{tracks: append([]netease.Track(nil), existing...), updated: map[int64]bool{}}
	var lyricFiles []string
	for _, source := range paths {
		if strings.EqualFold(filepath.Ext(source), ".lrc") {
			lyricFiles = append(lyricFiles, source)
			continue
		}
		track, err := localmedia.Read(source, artworkDir)
		if err != nil {
			result.failures = append(result.failures, filepath.Base(source)+"："+err.Error())
			continue
		}
		for _, previous := range result.tracks {
			if previous.LocalPath == track.LocalPath && previous.LocalLyricsPath != "" {
				track.LocalLyricsPath, track.LocalLyrics = previous.LocalLyricsPath, previous.LocalLyrics
				if text, readErr := localmedia.ReadLyrics(track.LocalLyricsPath); readErr == nil {
					track.LocalLyrics = text
				}
				break
			}
		}
		result.tracks = mergeLocalTracks(result.tracks, []netease.Track{track})
		result.updated[track.ID] = true
		result.audio++
	}
	for _, source := range lyricFiles {
		path, err := filepath.Abs(source)
		if err == nil {
			path, err = filepath.EvalSymlinks(path)
		}
		var text string
		if err == nil {
			text, err = localmedia.ReadLyrics(path)
		}
		if err != nil {
			result.failures = append(result.failures, filepath.Base(source)+"："+err.Error())
			continue
		}
		index, matches := -1, 0
		stem := strings.TrimSuffix(path, filepath.Ext(path))
		for candidate, track := range result.tracks {
			audioStem := strings.TrimSuffix(track.LocalPath, filepath.Ext(track.LocalPath))
			if strings.EqualFold(audioStem, stem) {
				index, matches = candidate, 1
				break
			}
			if strings.EqualFold(filepath.Base(audioStem), filepath.Base(stem)) {
				index, matches = candidate, matches+1
			}
		}
		if matches == 0 && len(lyricFiles) == 1 {
			for candidate, track := range result.tracks {
				if currentPath != "" && track.LocalPath == currentPath {
					index, matches = candidate, 1
					break
				}
			}
		}
		if matches != 1 {
			result.failures = append(result.failures, filepath.Base(source)+"：请先导入对应音频，或播放要添加歌词的本地歌曲；同名歌曲请将歌词放在音频旁")
			continue
		}
		result.tracks[index].LocalLyricsPath, result.tracks[index].LocalLyrics = path, text
		result.updated[result.tracks[index].ID] = true
		result.lyrics++
	}
	return result
}

func (a *app) importLocalFiles(paths []string) {
	if a.localImporting {
		a.toast = "正在导入，请稍候再添加文件"
		return
	}
	path, err := localLibraryPath()
	if err != nil {
		a.toast = err.Error()
		return
	}
	a.localImporting, a.localError = true, ""
	a.resetWallpaperTimer()
	existing := append([]netease.Track(nil), a.localTracks...)
	currentPath := ""
	if track := a.pl.Current(); track != nil {
		currentPath = track.LocalPath
	}
	go func() {
		result := readLocalImports(paths, existing, currentPath, filepath.Join(filepath.Dir(path), "local-artwork"))
		var saveErr error
		if result.audio+result.lyrics > 0 {
			saveErr = saveLocalLibrary(path, result.tracks)
		}
		a.post(func() {
			a.localImporting = false
			a.wallpaperInput()
			if saveErr != nil {
				a.localError = "音乐库保存失败：" + saveErr.Error()
			} else {
				a.localTracks = result.tracks
				for _, track := range result.tracks {
					if result.updated[track.ID] {
						a.pl.UpdateLocalLyrics(track.LocalPath, track.LocalLyricsPath, track.LocalLyrics)
					}
				}
				a.pl.SetLocalLibrary(a.localTracks)
				a.lyricAt, a.lyricTrack = -1, 0
				a.localError = strings.Join(result.failures, "\n")
				if result.lyrics > 0 {
					a.toast = fmt.Sprintf("已更新 %d 首歌曲的歌词", result.lyrics)
				} else if result.audio > 0 {
					a.toast = fmt.Sprintf("已导入 %d 首歌曲", result.audio)
				}
			}
			if result.audio > 0 || len(result.failures) > 0 || saveErr != nil {
				a.open(route{page: pageLocal})
			}
		})
	}()
}

func (a *app) localPage(c *ui.Context) {
	header := func(c *ui.Context) {
		a.heading(c, "本地音乐", fmt.Sprintf("%d 首 · 支持 MP3、FLAC、内嵌封面与同名 LRC", len(a.localTracks)))
		label := "导入音乐或歌词…"
		if a.localImporting {
			label = "正在导入…"
		}
		ui.Button(c, label).Disabled(a.localImporting).OnClick(a.chooseLocalAudio).AlignSelf(ui.Start).Margin(0, 0, 18, 0)
		if a.localError != "" {
			ui.Text(c, a.localError).FontSize(13).TextColor(a.pal.accent).Margin(0, 0, 16, 0)
		}
	}
	if len(a.localTracks) > 0 {
		a.trackRows(c, a.localTracks, true, header)
		return
	}
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom+32, pagePad).Children(func() {
		header(c)
		ui.Text(c, "将音频或 LRC 拖到窗口，或按 ⌘O 选择文件。歌词按同名音频匹配，单独导入也可用于当前播放的本地歌曲。").FontSize(14).TextColor(a.pal.muted)
	})
}
