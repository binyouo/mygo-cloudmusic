package app

import "netease-music-mygo/internal/mobileplayback"

func init() {
	connectNativePlayback = mobileplayback.Connect
}
