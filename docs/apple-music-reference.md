# Apple Music 播放体验参考

参考用户提供的 73 秒 iPad 播放视频，重点检查 34–39 秒的切歌，以及后续动态封面。

## GitHub 调研

- [AMLL](https://github.com/amll-dev/applemusic-like-lyrics)，检视时 HEAD 为 `6911611dd2f0fbd621d95a594ae048fd3474bab8`。根许可证为 AGPL-3.0，与本项目的 AGPL-3.0 一致。查看了 `packages/core/src/bg-render/mesh-renderer/index.ts` 与 `packages/core/src/lyric-player/dom/lyric-line.ts`：动态网格背景、图像及视频资源切换、歌词的缩放/透明度/模糊/延迟。这些是设计和算法方向参考，没有引入 WebView 或复制 TypeScript 代码。
- [Cider 1](https://github.com/ciderapp/Cider)：AGPL-3.0，Electron/Vue 的完整 Apple Music 客户端，可参考整体交互；本次没有引入其代码。
- [Mixxx Auto DJ](https://github.com/mixxxdj/mixxx/blob/main/src/library/autodj/autodjprocessor.cpp)：阅读了根据 intro/outro cue 决定混音时长的路径，作为音频规划参考，没有引入 C++ 代码。
- [Apple 官方说明](https://support.apple.com/en-ie/105067)区分动态的 AutoMix 与固定秒数的 Crossfade；同专辑顺序播放、曲风或速度不合适时，AutoMix 也可能保留自然衔接。

## 本次实现

- 自动及按钮、键盘、队列切歌均使用封面原位叠化，实际混音时跟随音频进度；鼠标悬停倾斜、反光、放大、拖拽深度摆动和甩动切歌保留。图片加载中保留上一张，无图或加载失败恢复占位。
- 封面采色后用原生着色器绘制缓慢流动的四色背景。暂停和减少动态效果时停止运动。
- 动态 WebP、GIF 使用原生 Go 解码，合成帧的偏移、透明度与 disposal；保留帧有 24 MiB 预算。动画使用独立位图，原高清静态封面和完整构图保留；GIF 在解码前检查帧数与像素预算。
- 歌名入场、歌词远近虚化、边缘渐隐；提前加载下一首的封面与歌词。本地及导入的歌词优先，云端预加载不能覆盖它们；播客字幕保留原有清晰显示。
- 自适应衔接：参考 [Music Assistant Smart Fades](music-assistant-smart-fades.md)，完整音频分析提供拍点、下拍、人声活动、调性和频段能量，MA 原版候选规划器选择衔接方案。原生 Go 后台执行分频交接、保音高变速、高通过滤或回声收尾；下一首始终原速。前奏入口与尾奏结束点遵循 MA 的保护策略；预处理未就绪时保留已有轻量／自然衔接。
- 音频仍在同一个输出中进行等功率交叉混音，封面混合进度跟随实际听到的音频位置。设置提供 AutoMix 和无缝衔接两种模式；同一专辑、重复曲目和播客保留原有样本边界。用户拖动进度明确定位当前歌曲，取消冲突的混音预读；旧设置中明确选择无缝的用户不因新增选项被自动开启混音。

## 明确边界

苹果专用的动态视频封面不包含在网易云的静态封面接口中，本次没有抓取、打包或伪造这些素材。支持的是音源提供的动态图片与静态封面的轻微运动。

自适应衔接已使用 Rubber Band 进行保持音高的变速。现已引入 Music Assistant 的 Beat This 下拍、FireRed 人声活动和 S-KEY 调性分析及完整候选规划，仍不包含 stem 人声分离，也不等同于苹果的专有 AutoMix。节奏、拍号或人声条件不合适时不会强行对拍。
