# 音频

`audio` 包可以在 Go 中播放声音：预先解码到内存的音效、流式播放的音乐，以及应用自行合成的声音。它在 Go 中混合所有正在播放的音频，再送到系统音频输出，无需 cgo，适用于原生 UI 和网页应用，支持 MyGo 的所有运行平台，包括浏览器。

```go
import (
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/vorbis" // Ogg Vorbis
)

//go:embed sounds
var sounds embed.FS

data, _ := sounds.ReadFile("sounds/hoe.wav")
hoe, err := audio.DecodeSound(data) // decoded once

hoe.Play() // as often as it sounds, over itself

f, _ := sounds.Open("sounds/spring.ogg")
music, err := audio.OpenStream(f.(io.ReadSeeker)) // decoded as it plays
music.SetLoop(true).SetVolume(0.6).Play()
```

## 声音与流

`Sound` 保存已解码的采样，用于经常播放的音效：每次 `Play` 都会新建一个播放器，所以同一个声音可以叠加播放；`NewPlayer` 返回一个已暂停的播放器，方便在播放前设置好：

```go
hoe.NewPlayer().SetPan(-0.4).SetSpeed(0.95 + rand.Float64()*0.1).Play()
```

`OpenStream` 在播放时提前约一秒解码文件，适用于音乐和其他长音频；`NewPlayer` 对任意 `Decoder` 也是如此。流的播放器用完后要关闭：关闭它就会关闭文件。声音的播放器则无需关闭。

## 播放器

`Player` 可以播放、暂停（`Pause`），并回到开头（`Stop`）；播完后再次 `Play` 会从开头重放。`Seek` 调整播放位置，`Position` 和 `Duration` 给出当前位置和时长，`Playing` 在播放结束时变为 false，游戏可以据此等待音效结束；`OnEnd` 会在此时通过独立的 goroutine 调用回调，例如让播放列表接着播放下一首（循环播放时不会触发结束）。

`Position` 表示播放器实际被听到的位置。输出混音会提前于扬声器发声（见[输出](#输出)），`Position` 也就比已混音的内容滞后同样的时间，因此歌词、进度条或由它绘制的可视化能与声音保持同步：`Pause` 之后，输出还在播放已缓冲的内容，它也会继续前进；而 `Play` 之后，它会等到播放器真正发声。`Seek` 和 `Stop` 则会立刻移动它。

| 设置 | 效果 |
|---|---|
| `SetVolume(v)` | 增益：1 为原始音量，0 为静音 |
| `SetPan(p)` | 从左扬声器（-1）移到右扬声器（1）：单声道保持总功率，立体声则同时作用于两个声道，与 Web Audio 的 `StereoPannerNode` 相同 |
| `SetSpeed(s)` | 像改变唱片转速一样同时改变速度和音高：2 表示两倍速，音高提高一个八度 |
| `SetPreservesPitch(true)` | 在任何速度下保持音高，如同 HTML 媒体的 `preservesPitch`：1.5× 的语音仍是录制时的嗓音 |
| `SetLoop(true)` | 在结尾重新开始，没有间隙 |
| `SetLoopRange(start, end)` | 从 `end` 循环回 `start`：一首曲子的前奏播放一次，然后是它的循环（`end` 为 0 表示结尾） |

设置方法都返回播放器，以便链式调用。音量变化、暂停和继续都在 5 ms 内渐变，所以不会有咔嗒声。`audio.SetVolume` 设置所有声音的音量。

`SetSpeed` 通过重采样改变音高，所以音高随速度变化，游戏里经常播放的音效正依赖这一点。播客、讲座或语音消息播放器在 1.25×、1.5× 或 2× 时希望人声保持原样：用 `SetPreservesPitch(true)`，播放器会改用时间伸缩（WSOLA，用 Go 实现），播放约 30 ms 的片段，并在波形吻合处交叉淡化，从而跳过或重复整个周期，音高保持不变：

```go
podcast, err := audio.OpenStream(f)
podcast.SetPreservesPitch(true).SetSpeed(1.5).Play()
```

它适合语音和其他有音高的声音，适用速度约为 0.5× 到 4×，但鼓声这类突然起音的声音会略显模糊；每个播放器在伸缩时约占用 1% 的单核算力，不保持音高的播放器则没有额外开销。速度变化、跳转、循环以及开启或关闭它都不会中断，`Position` 和 `OnEnd` 与重采样时一样，仍按源的时间计算。

## 媒体键与“正在播放”

`mygo.NowPlaying.Control` 让媒体键和系统的媒体控制（macOS 的控制中心、Windows 的音量浮出控件、Linux 上的 MPRIS 客户端）能够播放、暂停、停止和定位播放器，并在那里显示其状态；见 [正在播放](native.md#正在播放和媒体键)：

```go
release := mygo.NowPlaying.Control(music)
mygo.NowPlaying.Set(mygo.NowPlayingInfo{Title: "Spring", Artist: "Vivaldi", Commands: mygo.MediaNext})
music.Play()
mygo.NowPlaying.Sync() // after playing it from the app
```

## 格式

WAV（8 到 32 位整数 PCM，32 或 64 位浮点）开箱即支持。要启用某种格式，只需像包 `image` 那样，导入对应的包以触发其副作用：

| 包 | 格式 | 解码器 |
|---|---|---|
| `audio` | WAV | 内置 |
| `audio/vorbis` | Ogg Vorbis | [jfreymuth/oggvorbis](https://github.com/jfreymuth/oggvorbis) |
| `audio/mp3` | MP3 | [hajimehoshi/go-mp3](https://github.com/hajimehoshi/go-mp3) |
| `audio/flac` | FLAC，也支持 MP4 容器中的 FLAC | [mewkiz/flac](https://github.com/mewkiz/flac) |
| `audio/aac` | AAC-LC：支持 MP4（.m4a）和 ADTS（.aac） | [tphakala/go-aac](https://github.com/tphakala/go-aac)，采用 LGPL 许可 |
| `audio/mp4` | MP4（.m4a、.mp4、.m4s），含 Apple Lossless（ALAC） | 内置 |

以 ID3 标签开头的文件（MP3 如此，部分 FLAC 和 AAC 文件也如此），按标签之后的格式处理。

循环播放的音乐最好用 Ogg Vorbis：MP3 文件的开头和结尾带有编码器的填充，即几毫秒静音；除非用 `SetLoopRange` 排除，否则循环时也会播放这段静音。

```go
// 12.5 s of intro, then bars from 12.5 s to 1 min 40 s, again and again.
music.SetLoopRange(12500*time.Millisecond, 100*time.Second).Play()
```

`RegisterFormat` 可以添加你自己的格式；你写的 `Decoder` 能边生成边播放应用自己合成的声音，比如合成器：

```go
type tone struct{ pos int64 }

func (t *tone) SampleRate() int              { return 48000 }
func (t *tone) Channels() int                { return 1 }
func (t *tone) Len() int64                   { return -1 } // endless
func (t *tone) SetPosition(f int64) error    { t.pos = f; return nil }
func (t *tone) Read(buf []float32) (int, error) {
	for i := range buf {
		buf[i] = 0.2 * float32(math.Sin(2*math.Pi*440*float64(t.pos)/48000))
		t.pos++
	}
	return len(buf), nil
}

audio.NewPlayer(&tone{}).Play()
```

`NewSound` 接收应用一次性计算好的采样。

### MP4

`audio/mp4` 包负责读取容器，既支持完整的 .m4a 文件，也支持 DASH 流所用的分片文件（.m4s）；分片文件在全局媒体头之后，使用独立的分片头描述各个数据包。数据包由相应的编解码器解码：内置支持 Apple Lossless，导入 `audio/aac` 或 `audio/flac` 可为 MP4 添加 AAC 或 FLAC 支持，这两个包会自动导入 `audio/mp4`：

```go
import _ "github.com/egoist/mygo/audio/aac" // .m4a of AAC, and .aac

f, _ := os.Open("song.m4a")
music, err := audio.OpenStream(f)
```

它边播放边读取数据包，因此仍在传输中的文件（比如网络流）也能随到随播，前提是读取器会等待所需的字节到达：分片文件从第一个分片开始播放，时长取自片段索引（`sidx`）；没有索引的文件则在打开时通读一遍以确定时长。编辑列表指定跳过的编码器预填充和最后一个包的尾部填充都会被跳过，因此音轨时长与编码前一致。跳转会落在所请求的帧上；AAC 还会解码前一个包，因为它的声音与该帧重叠。

`mp4.ReadTrack` 无需解码即可从全局媒体头读取音轨信息，适合音乐库扫描文件时使用：包括音轨的编解码器、配置、时长，以及已注册的编解码器是否支持解码（`Decodable`）。`mp4.RegisterCodec` 可按采样条目类型注册自定义编解码器：

```go
mp4.RegisterCodec("Opus", func(config []byte) (mp4.Codec, error) {
	return newOpusCodec(config) // config is the boxes within the sample entry: dOps
})
```

不支持解码 HE-AAC（AAC+，带 SBR）：这类文件打开会失败，`Decodable` 也会这样表明。go-aac 采用 LGPL 2.1 或更高版本的许可，这一点与 MyGo 不同：导入 `audio/aac` 的应用会链接它，并须遵守 LGPL 对该库的条款；不导入的应用则不受影响。

## 输出

音频输出在首次播放时打开，全部停止五秒后关闭，因此不播放声音的应用不会占用音频设备。输出采用 48 kHz 立体声，提前混音约 40 ms；其他采样率的播放器在播放时重采样。有效采样率高于 48 kHz 的音频——例如 88.2、96 或 192 kHz 的音频，或加速播放的音频——会先经过低通滤波：20 kHz 以内的频率响应保持平坦，24 kHz 及以上衰减 80 dB，避免这些高频成分混叠为可听音调；每个这样的 96 kHz 播放器约占用 1% 的单核算力。

`audio.SetLatency` 设置混音提前多少，与 Web Audio 的 `latencyHint` 相同：

| 延迟 | 提前 | 用于 |
|---|---|---|
| `LatencyInteractive`（默认） | 约 40 ms | 游戏和界面：声音收到指令就立即开始、停止和变化 |
| `LatencyPlayback` | 约 250 ms | 音乐播放器：在系统暂时阻塞应用时维持播放，例如 macOS 切换外观时可能阻塞应用长达 0.2 s；此时 40 ms 的缓冲会耗尽，导致可听的停顿 |

使用 `LatencyPlayback` 时，播放、暂停、跳转和音量变化都会晚这么久才能听到；播放器的 `Position` 跟随实际听到的内容。要在播放前设置：已经打开的输出会重新打开，并丢弃其中缓冲的内容。

| 平台 | 输出 |
|---|---|
| macOS | 默认输出设备上的 Audio Queue，会跟随默认输出的切换 |
| Linux | PulseAudio 的简单 API（PipeWire 也能提供），否则用 ALSA 的默认设备 |
| Windows | 共享模式的 WASAPI，默认输出变化时重新打开 |
| 浏览器 | 一个 AudioWorklet；页面只有在用户与其交互后才能播放音频，所以它会在第一次点击或按键时启动 |

如果没有输出能打开，比如机器没有音频设备，播放器仍会按时播放，只是听不到声音，原因由 `audio.Err` 说明。每个函数和方法都可以从任何 goroutine 安全调用。

[examples/piano](../examples/piano) 是一架用合成音色做成的钢琴，可用指针和键盘演奏，带延音踏板、让琴弦淡出的制音器，以及几首自动演奏的曲子。
