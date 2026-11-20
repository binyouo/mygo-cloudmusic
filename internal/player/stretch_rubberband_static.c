//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))

#include "rubberband_link.h"
#include "rubberband/rubberband-c.h"

int cloudmusic_rubberband_engine(RubberBandState state) {
    return rubberband_get_engine_version(state);
}
