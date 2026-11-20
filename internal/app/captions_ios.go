package app

import "netease-music-mygo/internal/speech"

func init() {
	systemCaptions = true
	transcriptModel = speech.DeviceModelName
	captionsAvailable = func() bool { return true }
	modelInstalled = speech.DeviceInstalled
	downloadModel = speech.EnableDevice
	removeModel = speech.RemoveDevice
	newTranscriber = func(string) (transcriber, error) { return speech.DeviceRecognizer{}, nil }
}
