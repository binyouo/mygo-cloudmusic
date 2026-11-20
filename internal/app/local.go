package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		a.localTracks, err = readLocalLibrary(path, a.phone)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.localError = "本地音乐库读取失败：" + err.Error()
	}
	a.pl.SetLocalLibrary(a.localTracks)
	if a.settings.LocalTagsVersion < 1 && len(a.localTracks) > 0 {
		paths := make([]string, 0, len(a.localTracks))
		for _, track := range a.localTracks {
			paths = append(paths, track.LocalPath)
		}
		a.readLocalFiles(paths, true, 0)
	}
}

func readLocalLibrary(path string, phone bool) ([]netease.Track, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tracks []netease.Track
	if err := json.Unmarshal(data, &tracks); err != nil {
		return nil, err
	}
	if phone {
		tracks = mapPhoneTrackPaths(tracks, filepath.Dir(path), resolvePhonePath)
	}
	return tracks, nil
}

func saveLocalLibrary(path string, tracks []netease.Track, phone bool) error {
	if phone {
		tracks = mapPhoneTrackPaths(tracks, filepath.Dir(path), relativePhonePath)
	}
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
	a.chooseLocalFiles(0)
}

var pickLocalFiles = mygo.Dialog.Open

func (a *app) chooseLocalFiles(lyricsFor int64) {
	if a.localPicking || a.localImporting {
		return
	}
	a.showMain()
	a.localPicking = true
	options := mygo.OpenDialogOptions{
		Parent: a.win, Title: "导入本地音乐", Multiple: true,
		Filters: []mygo.FileFilter{{Name: "音乐与歌词", Extensions: []string{"mp3", "flac", "ogg", "lrc"}}},
	}
	if lyricsFor != 0 {
		options.Title, options.Multiple = "导入歌词", false
		options.Filters = []mygo.FileFilter{{Name: "LRC 歌词", Extensions: []string{"lrc"}}}
	}
	pick := pickLocalFiles
	go func() {
		paths, err := pick(options)
		a.post(func() {
			a.localPicking = false
			if err != nil {
				a.toast = "无法打开文件选择器：" + err.Error()
				return
			}
			if len(paths) > 0 {
				a.readLocalFiles(paths, false, lyricsFor)
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

type localImportOptions struct {
	currentPath string
	lyricsFor   int64
	phone       bool
	pickedFrom  map[string]string // staged path to the path returned by Files
}

func readLocalImports(paths []string, existing []netease.Track, artworkDir string, options localImportOptions) localImportResult {
	result := localImportResult{tracks: append([]netease.Track(nil), existing...), updated: map[int64]bool{}}
	var lyricFiles []string
	var pickedAudio []netease.Track
	for _, source := range paths {
		if strings.EqualFold(filepath.Ext(source), ".lrc") {
			lyricFiles = append(lyricFiles, source)
			continue
		}
		if options.lyricsFor != 0 {
			result.failures = append(result.failures, filepath.Base(source)+"：请选择 LRC 歌词文件")
			continue
		}
		track, err := localmedia.Read(source, artworkDir)
		if err != nil {
			result.failures = append(result.failures, filepath.Base(source)+"："+err.Error())
			continue
		}
		for _, previous := range result.tracks {
			if previous.LocalPath != track.LocalPath {
				continue
			}
			// An imported file may have moved with the phone's sandbox.
			track.ID = previous.ID
			if previous.LocalLyricsPath != "" {
				track.LocalLyricsPath, track.LocalLyrics = previous.LocalLyricsPath, previous.LocalLyrics
				if text, readErr := localmedia.ReadLyrics(track.LocalLyricsPath); readErr == nil {
					track.LocalLyrics = text
				}
			}
			break
		}
		result.tracks = mergeLocalTracks(result.tracks, []netease.Track{track})
		result.updated[track.ID] = true
		result.audio++
		if picked := options.pickedFrom[source]; picked != "" {
			track.LocalPath = picked
			pickedAudio = mergeLocalTracks(pickedAudio, []netease.Track{track})
		}
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
		if options.lyricsFor != 0 {
			for candidate, track := range result.tracks {
				if track.ID == options.lyricsFor && track.LocalPath != "" {
					index, matches = candidate, 1
					break
				}
			}
		} else {
			// Files picked together are still related after each is copied
			// into its own content directory. Check them before older songs.
			if picked := options.pickedFrom[source]; picked != "" {
				candidate, n := matchLocalLyrics(picked, pickedAudio, "", true)
				matches = n
				if n == 1 {
					id := pickedAudio[candidate].ID
					index = slices.IndexFunc(result.tracks, func(track netease.Track) bool { return track.ID == id })
				}
			}
			if matches == 0 {
				currentPath := ""
				if len(lyricFiles) == 1 {
					currentPath = options.currentPath
				}
				index, matches = matchLocalLyrics(path, result.tracks, currentPath, options.phone)
			}
		}
		if matches != 1 {
			help := "请先导入对应音频，或播放要添加歌词的本地歌曲；同名歌曲请将歌词放在音频旁"
			if options.lyricsFor != 0 {
				help = "找不到要添加歌词的本地歌曲"
			} else if options.phone {
				help = "请先导入音频，再从歌曲的「···」选择「导入歌词…」"
			}
			result.failures = append(result.failures, filepath.Base(source)+"："+help)
			continue
		}
		result.tracks[index].LocalLyricsPath, result.tracks[index].LocalLyrics = path, text
		result.updated[result.tracks[index].ID] = true
		result.lyrics++
	}
	return result
}

func matchLocalLyrics(path string, tracks []netease.Track, currentPath string, uniqueExact bool) (index, matches int) {
	index = -1
	exact, exactMatches := -1, 0
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	for candidate, track := range tracks {
		audioStem := strings.TrimSuffix(track.LocalPath, filepath.Ext(track.LocalPath))
		if strings.EqualFold(audioStem, stem) {
			if !uniqueExact {
				return candidate, 1
			}
			exact, exactMatches = candidate, exactMatches+1
		}
		if strings.EqualFold(filepath.Base(audioStem), filepath.Base(stem)) {
			index, matches = candidate, matches+1
		}
	}
	if exactMatches > 0 {
		return exact, exactMatches
	}
	if matches == 0 && currentPath != "" {
		for candidate, track := range tracks {
			if track.LocalPath == currentPath {
				return candidate, 1
			}
		}
	}
	return index, matches
}

func (a *app) importLocalFiles(paths []string) {
	a.readLocalFiles(paths, false, 0)
}

// readLocalFiles imports explicit files or refreshes stored tags in the background.
func (a *app) readLocalFiles(paths []string, refresh bool, lyricsFor int64) {
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
	phone := a.phone
	go func() {
		root := filepath.Dir(path)
		var copied, failures []string
		var pickedFrom map[string]string
		if phone && !refresh {
			var durable []string
			pickedFrom = make(map[string]string, len(paths))
			for _, source := range paths {
				saved, created, err := keepPhoneImport(filepath.Join(root, "local-media"), source)
				if err != nil {
					failures = append(failures, filepath.Base(source)+"："+err.Error())
					continue
				}
				durable = append(durable, saved)
				pickedFrom[saved] = source
				if created {
					copied = append(copied, saved)
				}
			}
			paths = durable
		}
		result := readLocalImports(paths, existing, filepath.Join(root, "local-artwork"), localImportOptions{
			currentPath: currentPath, lyricsFor: lyricsFor, phone: phone, pickedFrom: pickedFrom,
		})
		result.failures = append(failures, result.failures...)
		var saveErr error
		if result.audio+result.lyrics > 0 {
			saveErr = saveLocalLibrary(path, result.tracks, phone)
		}
		if len(copied) > 0 {
			retained := make(map[string]bool)
			if saveErr == nil {
				for _, tr := range result.tracks {
					retained[tr.LocalPath], retained[tr.LocalLyricsPath] = true, true
				}
			}
			for _, path := range copied {
				if !retained[path] {
					os.Remove(path)
					os.Remove(filepath.Dir(path))
				}
			}
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
				if current := a.pl.Current(); current != nil && result.updated[current.ID] {
					a.lyricAt, a.lyricTrack = -1, 0
				}
				a.localError = strings.Join(result.failures, "\n")
				if refresh {
					a.settings.LocalTagsVersion = 1
					a.settings.save()
				} else if result.lyrics > 0 {
					a.toast = fmt.Sprintf("已更新 %d 首歌曲的歌词", result.lyrics)
				} else if result.audio > 0 {
					a.toast = fmt.Sprintf("已导入 %d 首歌曲", result.audio)
				}
			}
			if lyricsFor != 0 && a.localError != "" {
				a.toast = a.localError
			} else if !refresh && (result.audio > 0 || len(result.failures) > 0 || saveErr != nil) {
				a.open(route{page: pageLocal})
			}
		})
	}()
}

func (a *app) localPage(c *ui.Context) {
	header := func(c *ui.Context) {
		a.heading(c, "本地音乐", fmt.Sprintf("%d 首 · 支持 MP3、FLAC、Ogg、内嵌封面与同名 LRC", len(a.localTracks)))
		label := "导入音乐或歌词…"
		if a.localImporting {
			label = "正在导入…"
		}
		button := a.settingButton(c, "import", label, a.localPicking || a.localImporting).
			OnClick(a.chooseLocalAudio).AlignSelf(ui.Start).Margin(0, 0, 18, 0)
		if a.phone {
			button.MinHeight(44)
		}
		if a.localError != "" {
			ui.Text(c, a.localError).FontSize(13).TextColor(a.pal.accent).Margin(0, 0, 16, 0)
		}
	}
	if len(a.localTracks) > 0 {
		a.trackRows(c, a.localTracks, true, header)
		return
	}
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top+pageGap, a.pagePadding(), bottom+32, a.pagePadding()).Children(func() {
		header(c)
		help := "将音频或 LRC 拖到窗口，或按 ⌘O 选择文件。歌词按同名音频匹配，单独导入也可用于当前播放的本地歌曲。"
		if a.phone {
			help = "从文件选取音频或 LRC。歌词按同名音频匹配。"
		}
		ui.Text(c, help).FontSize(14).TextColor(a.pal.muted)
	})
}
