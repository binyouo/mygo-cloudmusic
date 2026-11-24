package speech

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

const DeviceModelName = "ios-speech-v1"

const deviceSpeechPlugin = "cloudmusic_speech"

var recognitionID atomic.Uint64

func DeviceInstalled(root string) bool {
	_, err := os.Stat(filepath.Join(root, DeviceModelName))
	return err == nil
}

func EnableDevice(ctx context.Context, root string, _ func(int64, int64)) error {
	call, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := mygo.Native(deviceSpeechPlugin).Call(call, "authorize", nil, nil); err != nil {
		return deviceSpeechError(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, DeviceModelName), nil, 0600)
}

func RemoveDevice(root string) error {
	err := os.Remove(filepath.Join(root, DeviceModelName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type DeviceRecognizer struct{}

func (DeviceRecognizer) Close() {}

func (DeviceRecognizer) Transcribe(ctx context.Context, src Source, from, until time.Duration, said func(Segment), heard func(time.Duration)) error {
	temp, err := mygo.App.Path(mygo.PathTemp)
	if err != nil {
		return err
	}
	return transcribeChunks(ctx, temp, src, from, until, recognizeDeviceChunk, said, heard)
}

func recognizeDeviceChunk(ctx context.Context, path string) ([]Segment, error) {
	var result []struct {
		Start, End float64
		Text       string
	}
	id := recognitionID.Add(1)
	native := mygo.Native(deviceSpeechPlugin)
	call, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	err := native.Call(call, "recognize", struct {
		ID   uint64
		Path string
	}{id, path}, &result)
	if call.Err() != nil {
		cancelCall, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = native.Call(cancelCall, "cancel", struct{ ID uint64 }{id}, nil)
		return nil, call.Err()
	}
	if err != nil {
		return nil, deviceSpeechError(err)
	}
	segments := make([]Segment, 0, len(result))
	for _, s := range result {
		segments = append(segments, Segment{
			Start: time.Duration(s.Start * float64(time.Second)),
			End:   time.Duration(s.End * float64(time.Second)),
			Text:  s.Text,
		})
	}
	return segments, nil
}

func deviceSpeechError(err error) error {
	var native *mygo.NativeError
	if errors.As(err, &native) {
		switch native.Code {
		case "Unauthorized":
			return errors.New("请在 iOS 设置中允许语音识别，然后重试")
		case "Unsupported":
			return errors.New("当前设备或系统语言不支持离线语音识别")
		case "Unavailable":
			return errors.New("系统语音识别暂不可用，请稍后重试")
		}
	}
	return err
}
