//go:build (!darwin && !linux) || (ios && !cgo) || android

package player

func openTailStretcher() (*rubberBandAPI, error) {
	return nil, errStretchUnavailable
}
