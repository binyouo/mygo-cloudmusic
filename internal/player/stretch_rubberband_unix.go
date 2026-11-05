//go:build darwin || linux

package player

import (
	"fmt"

	"github.com/ebitengine/purego"
)

func openRubberBandLibrary(path string) (*rubberBandAPI, error) {
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	api := &rubberBandAPI{handle: handle}
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
