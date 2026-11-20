//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))

package player

/*
#cgo CPPFLAGS: -I${SRCDIR}/../../build/rubberband-4.0.0 -I${SRCDIR}/rubberband
#cgo CFLAGS: -fmodules
#cgo CXXFLAGS: -std=c++11
#include "rubberband/rubberband-c.h"
int cloudmusic_rubberband_engine(RubberBandState state);
*/
import "C"

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

func openTailStretcher() (*rubberBandAPI, error) {
	// Keep the same synchronous C calls and planar buffer layout as desktop.
	// cgo resolves the addresses in the app's archive at link time.
	api := &rubberBandAPI{}
	purego.RegisterFunc(&api.new, uintptr(unsafe.Pointer(C.rubberband_new)))
	purego.RegisterFunc(&api.delete, uintptr(unsafe.Pointer(C.rubberband_delete)))
	purego.RegisterFunc(&api.engine, uintptr(unsafe.Pointer(C.cloudmusic_rubberband_engine)))
	purego.RegisterFunc(&api.setTimeRatio, uintptr(unsafe.Pointer(C.rubberband_set_time_ratio)))
	purego.RegisterFunc(&api.startPad, uintptr(unsafe.Pointer(C.rubberband_get_preferred_start_pad)))
	purego.RegisterFunc(&api.startDelay, uintptr(unsafe.Pointer(C.rubberband_get_start_delay)))
	purego.RegisterFunc(&api.setMaxBlock, uintptr(unsafe.Pointer(C.rubberband_set_max_process_size)))
	purego.RegisterFunc(&api.process, uintptr(unsafe.Pointer(C.rubberband_process)))
	purego.RegisterFunc(&api.available, uintptr(unsafe.Pointer(C.rubberband_available)))
	purego.RegisterFunc(&api.retrieve, uintptr(unsafe.Pointer(C.rubberband_retrieve)))
	return api, nil
}
