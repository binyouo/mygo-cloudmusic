# 网易云音乐 · MyGo

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方网易云音乐桌面客户端，原生界面，无 WebView。

## 功能

- 扫码登录，每日推荐、歌单、排行榜与搜索
- 歌手、专辑和用户主页，支持关注歌手
- 播客收听、订阅与本地字幕
- 私人 FM、心动模式，支持随机和循环播放
- 边下边播、本地缓存、无损音质与音量均衡
- 恢复上次的播放队列与进度
- 逐字歌词、翻译、律动视图与歌曲评论
- 全屏播放屏保（⌘T）
- 导入本地 MP3、FLAC 和 LRC（⌘O 或拖入）
- 刘海播放器、macOS 菜单栏歌词与播放控制
- 自定义应用内及全局播放快捷键（macOS）
- 尝试从波点音乐匹配无法播放的歌曲
- 浅色、深色及跟随系统

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
```

## 致谢

- [MyGo](https://github.com/egoist/mygo)：原生应用框架
- [NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi)：登录与 API
- [UnblockNeteaseMusic/server](https://github.com/UnblockNeteaseMusic/server)：波点音源，见 `LICENSE.unblock` 与 `LICENSE.unblock-GPL`
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别
- [Feishin](https://github.com/jeffvli/feishin/blob/8d12eae8a1551364442461bac02d99d78c185820/src/renderer/hooks/use-fullscreen-auto-open.ts)、[Matrix Screensaver](https://github.com/WadeSellers/matrix-screensaver)：播放屏保
- [dhowden/tag](https://github.com/dhowden/tag/tree/3d75831295e8)、[APlayer](https://github.com/DIYgod/APlayer/blob/master/src/js/lrc.js)：本地音乐与歌词

本项目与网易云音乐官方无关，音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
