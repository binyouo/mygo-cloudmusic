//go:build ios && cgo

package player

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

func init() { analyzeDeviceMix = coreMLMixProfile }

var coreMLMixModels = sync.OnceValues(func() (deviceMixModelBundle, error) {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return deviceMixModelBundle{}, err
	}
	return deviceMixModels(filepath.Join(resources, "automix")), nil
})

func coreMLMixProfile(ctx context.Context, dl *download, ms int64) (deviceMixProfile, error) {
	models, err := coreMLMixModels()
	if err != nil {
		return deviceMixProfile{}, err
	}
	temp, err := mygo.App.Path(mygo.PathTemp)
	if err != nil {
		return deviceMixProfile{}, err
	}
	cache := ""
	if base, err := mygo.App.Path(mygo.PathCache); err == nil {
		cache = filepath.Join(base, "smart-fades-coreml-v1")
	}
	native := mygo.Native("cloudmusic_beat")
	return analyzeAppleMixProfile(ctx, dl, ms, temp, cache, models, func(ctx context.Context, method string, req deviceModelRequest, reply any) error {
		return native.Call(ctx, method, req, reply)
	})
}
