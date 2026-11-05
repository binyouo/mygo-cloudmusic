//go:build !darwin && !linux

package player

func openRubberBandLibrary(string) (*rubberBandAPI, error) {
	return nil, errStretchUnavailable
}
