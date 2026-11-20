//go:build (darwin && !ios && !(cloudmusic_static_rubberband && cgo)) || (linux && !android)

package player

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ebitengine/purego"
)

func openTailStretcher() (*rubberBandAPI, error) {
	var last error
	for _, path := range rubberBandLibraryPaths() {
		api, err := openRubberBandLibrary(path)
		if err == nil {
			return api, nil
		}
		last = err
	}
	return nil, fmt.Errorf("%w: %v", errStretchUnavailable, last)
}

// rubberBandLibraryPaths prefers the application bundle. Homebrew/system paths
// are development fallbacks; production packages must carry their own library.
func rubberBandLibraryPaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Frameworks"))
		switch runtime.GOOS {
		case "darwin":
			paths = append(paths, filepath.Join(dir, "librubberband.3.dylib"), filepath.Join(dir, "librubberband.2.dylib"), filepath.Join(dir, "librubberband.dylib"))
		case "linux":
			paths = append(paths, filepath.Join(dir, "librubberband.so.3"), filepath.Join(dir, "librubberband.so.2"), filepath.Join(dir, "librubberband.so"))
		}
	}
	switch runtime.GOOS {
	case "darwin":
		paths = append(paths,
			"/opt/homebrew/opt/rubberband/lib/librubberband.3.dylib",
			"/opt/homebrew/opt/rubberband/lib/librubberband.2.dylib",
			"/opt/homebrew/opt/rubberband/lib/librubberband.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.3.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.2.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.dylib")
	case "linux":
		paths = append(paths, "librubberband.so.3", "librubberband.so.2", "librubberband.so")
	}
	return paths
}

func openRubberBandLibrary(path string) (*rubberBandAPI, error) {
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	api := &rubberBandAPI{}
	for name, fn := range map[string]any{
		"rubberband_new":                     &api.new,
		"rubberband_delete":                  &api.delete,
		"rubberband_get_engine_version":      &api.engine,
		"rubberband_set_time_ratio":          &api.setTimeRatio,
		"rubberband_get_preferred_start_pad": &api.startPad,
		"rubberband_get_start_delay":         &api.startDelay,
		"rubberband_set_max_process_size":    &api.setMaxBlock,
		"rubberband_process":                 &api.process,
		"rubberband_available":               &api.available,
		"rubberband_retrieve":                &api.retrieve,
	} {
		symbol, err := purego.Dlsym(handle, name)
		if err != nil || symbol == 0 {
			purego.Dlclose(handle)
			return nil, fmt.Errorf("player: %s has no %s", path, name)
		}
		purego.RegisterFunc(fn, symbol)
	}
	return api, nil
}
