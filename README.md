# 网易云音乐 · MyGo

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方网易云音乐桌面客户端，原生界面，无 WebView。

## 功能

- 扫码登录，每日推荐、歌单、排行榜与搜索
- 歌手、专辑和用户主页，支持关注歌手
- 播客收听、订阅与本地字幕
- 私人 FM、心动模式，支持随机和循环播放
- 边下边播、本地缓存、无损音质与音量均衡
- 恢复上次的播放队列与进度
- 原位封面叠化、多色流动背景与歌词景深，支持动态 WebP/GIF 封面
- Smart Fades 式歌曲衔接：拍点规划、保持音高的渐进变速与分频交接；同专辑曲目与播客保留无缝衔接
- 逐字歌词、翻译、律动视图与歌曲评论
- 全屏播放屏保（⌘T）
- 导入本地 MP3、FLAC 和 LRC（⌘O 或拖入）
- 刘海播放器、macOS 菜单栏歌词与播放控制
- 自定义应用内及全局播放快捷键（macOS）
- VIP 测试功能：歌曲无法完整播放时尝试匹配波点音源，支持歌单、FM 与最近播放
- 浅色、深色及跟随系统

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
```

### iOS 本地开发

运行 iOS 模拟器需要完整的 Xcode 和 iOS 模拟器运行时。将仓库放在 T7 上时，脚本默认使用同盘的 `mygo-ios-dev` 保存开发缓存，并识别其中的 `Xcode.app`。可用 `CLOUDMUSIC_IOS_DEV_DIR` 指定开发目录。

macOS 15.6 可以使用 Xcode 26.3。把解压后的 `Xcode.app` 放进开发目录，脚本会为当前命令选用它。 [Apple 兼容性说明](https://developer.apple.com/xcode/system-requirements/) · [Xcode 下载](https://developer.apple.com/download/all/?q=Xcode%2026.3)

```bash
./scripts/mygo-ios.sh doctor                  # 检查 SDK 和模拟器
./scripts/mygo-ios.sh prepare-coreml          # 检查并准备自动混音模型
./scripts/mygo-ios.sh devices                 # 查看模拟器和已连接的 iPhone
./scripts/mygo-ios.sh dev -device <名称或UDID> # 启动指定模拟器并开发
./scripts/mygo-ios.sh build                   # 打包 iOS 模拟器版
./scripts/mygo-ios.sh dev -device <UDID>       # 用开发签名安装到已连接的 iPhone
./scripts/mygo-ios.sh build -platform ios     # 用分发签名构建 IPA
```

已启动模拟器时，可以直接运行 `./scripts/mygo-ios.sh dev`。如需将模拟器运行时的下载文件留在 T7，将 `DEVELOPER_DIR` 指向 T7 上的 `Xcode.app/Contents/Developer`，再运行 `xcodebuild -downloadPlatform iOS -exportPath <T7目录>`。安装后的运行时由 macOS 管理。 [Apple 的运行时下载和安装说明](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)

如果 MyGo 随包的 iOS 着色器库与源码不匹配，构建脚本会用 Xcode 的 Metal 工具链重新生成并缓存。缺少该组件时，运行 `xcodebuild -downloadComponent MetalToolchain` 安装。

真机签名使用自己的 Apple Developer App ID、证书和配置描述文件。在当前工作树的 `mygo.json` 设置所属团队的 `identifier`，并可用 `ios.team` 或 `ios.provisioningProfile` 指定签名配置。开发版会给标识符加 `.dev`，其开发配置描述文件需覆盖该标识符和测试设备。分发版使用原始标识符。

模拟器运行和构建会自动准备模型。默认使用 `mygo-ios-dev/coreml/verified-models`，也可以把 `CLOUDMUSIC_COREML_DIR` 指向 `scripts/smart_fades/export_coreml.py` 导出的目录。S-KEY 的预训练权重只授权用于非商业研究。 [S-KEY 权重许可](https://github.com/deezer/skey#-license)

macOS 版手机界面预览可以用 Command Line Tools 构建。预览包、设置、播放队列和缓存都保存在 `build/ios-preview`。

```bash
./scripts/mygo-ios.sh preview-build          # 构建并验签 macOS 预览包
./scripts/mygo-ios.sh preview                # 重建并在副屏打开预览
```

## 致谢

- [MyGo](https://github.com/egoist/mygo)：原生应用框架
- [NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi)：登录与 API
- [UnblockNeteaseMusic/server](https://github.com/UnblockNeteaseMusic/server)：波点音源，见 `LICENSE.unblock` 与 `LICENSE.unblock-GPL`
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画
- [Apple Music-like Lyrics](https://github.com/amll-dev/applemusic-like-lyrics)：流动背景与歌词景深的设计参考；原生 Go 实现
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别
- [Feishin](https://github.com/jeffvli/feishin/blob/8d12eae8a1551364442461bac02d99d78c185820/src/renderer/hooks/use-fullscreen-auto-open.ts)、[Matrix Screensaver](https://github.com/WadeSellers/matrix-screensaver)：播放屏保
- [dhowden/tag](https://github.com/dhowden/tag/tree/3d75831295e8)、[APlayer](https://github.com/DIYgod/APlayer/blob/master/src/js/lrc.js)：本地音乐与歌词

本项目与网易云音乐官方无关，音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
