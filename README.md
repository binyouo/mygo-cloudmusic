# 网易云音乐 · MyGo

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方网易云音乐桌面客户端，原生界面，无 WebView。

## 功能

- 扫码登录，登录态保存在系统凭据存储中
- 每日推荐、推荐歌单、排行榜、我的歌单
- 搜索单曲、歌手、专辑、用户与播客，输入时显示搜索建议，保留搜索历史
- 歌手页的热门与最新歌曲、专辑与动态，关注歌手；专辑页与用户页
- 播客：浏览与收听节目，订阅播客；在本机把节目转写成字幕（Qwen3-ASR），像歌词一样随播放滚动
- 私人 FM（推荐 / 熟悉 / 探索）与心动模式
- 边下边播，支持标准 / 较高 / 极高 / 无损（FLAC）/ Hi-Res 音质
- 听过的歌曲缓存在本机，再次播放无需等待
- 音量均衡，暂停与继续时淡出淡入，拖动进度无杂音
- 退出后再打开，从上次的播放队列与进度继续
- 逐字歌词与翻译，正在播放页的「律动」视图与歌曲评论
- 全屏播放屏保（⌘T）：在当前显示器渐显封面、动态背景与歌词，退出时淡回原窗口；macOS 可设置前台播放闲置 30 秒至 10 分钟后自动进入，默认 2 分钟，移动鼠标或按键退出
- 本地音乐：⌘O 或拖入 MP3 / FLAC / LRC，读取内嵌封面与同名歌词，支持 UTF-8、UTF-16、GBK；独立 LRC 按同名歌曲匹配，也可用于当前播放的本地歌曲，立即生效，播放时重新读取文件
- 刘海播放器：在屏幕顶部刘海处显示迷你播放器，悬停展开（在设置中开启）
- 菜单栏歌词（macOS）：展示宽度可调（100–300），短句居中、长句自动滚动，支持切歌、播放/暂停和喜欢歌曲（在设置中开启）
- 常用快捷键（macOS）：播放/暂停、上一首和下一首的应用内及全局按键均可在设置中自定义，支持恢复默认
- 网易云无法播放的歌曲可尝试从波点音乐匹配音源，匹配不到时仍无法播放
- 喜欢歌曲、播放队列、随机与循环播放
- 浅色 / 深色 / 跟随系统

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
```

### 本机 macOS 构建

频繁修改的本地版本使用固定的钥匙串辅助程序。macOS 的文件钥匙串仍会按二进制哈希区分自签名构建，仅固定证书不足以保持授权；脚本会复用同一份辅助程序，让主程序重编译后继续使用原有授权。登录态仍保存在系统钥匙串中，通过进程管道读写，不写入文件或命令行参数。

```bash
python3 scripts/build-local-macos.py
# 用临时凭据验证跨编译访问与调用者校验，全程禁用授权弹窗
python3 scripts/test-local-keychain.py
```

脚本复用 `build/darwin-<架构>/` 中已有应用的图标资源，重新编译、签名并验证后替换该目录下的应用，不创建备份，也不复制到 Applications。Apple Silicon 上的产物为 `build/darwin-arm64/MyGo云音乐.app`，首次改名构建会替换原来的 `MyGo 云音乐.app`。证书和私钥保存在本机登录钥匙串，默认名称为 `MyGo CloudMusic Local Signing`，也可通过 `MYGO_SIGNING_IDENTITY` 指定已有证书。辅助程序只接受同一证书签名、应用标识一致的父进程，且只能操作此应用的登录记录。脚本开启 hardened runtime；证书缺失或签名失败会停止。

本地版显示为 `MyGo云音乐`，应用标识为 `dev.egoist.cloudmusic.local`。设置、播放记录、本地曲库和模型继续保存在 `~/Library/Application Support/MyGo 云音乐（本地）/`，不随显示名称改变；歌曲缓存及钥匙串登录记录也与下载版分开。本地版通过此脚本更新，不启用远端内置更新；下载的官方版继续使用原目录与官方内置更新。

首次切换时，macOS 可能要求为 `MyGo Keychain` 授权一次，需输入本机登录密码并选择“始终允许”。辅助程序缓存在 `~/Library/Application Support/dev.egoist.cloudmusic.local/local-signing/`，每次构建原样复制进应用；请保留该目录和签名证书。修改辅助程序本身、换证书或清理缓存后，可能需要重新授权。

此流程需安装 Xcode Command Line Tools，用于本机使用，不替代发行版的 Developer ID 签名和公证。普通 `go tool mygo build` 不启用这个本机辅助程序；日常更新请使用上面的脚本。

## 致谢

- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi)：eapi 加密与登录流程的参考
- [UnblockNeteaseMusic/server](https://github.com/UnblockNeteaseMusic/server)：波点音乐音源协议的参考，见 `LICENSE.unblock` 与 `LICENSE.unblock-GPL`
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画效果的参考
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别
- [Feishin](https://github.com/jeffvli/feishin/blob/8d12eae8a1551364442461bac02d99d78c185820/src/renderer/hooks/use-fullscreen-auto-open.ts)：播放时按输入活动计时、自动进入播放界面的参考
- [Matrix Screensaver](https://github.com/WadeSellers/matrix-screensaver/blob/main/MatrixApp/MatrixWindow.swift)：以无边框窗口覆盖显示器的屏保方式；其 [MatrixSession](https://github.com/WadeSellers/matrix-screensaver/blob/main/MatrixApp/MatrixSession.swift) 提供了输入退出、鼠标抖动过滤和淡出生命周期的参考
- [dhowden/tag](https://github.com/dhowden/tag/tree/3d75831295e8)：本地 MP3 / FLAC 标签、封面与歌词读取
- [APlayer](https://github.com/DIYgod/APlayer/blob/master/src/js/lrc.js)：LRC 连续时间戳、重复行与行内时间标记处理的参考

本项目与网易云音乐官方无关，音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
