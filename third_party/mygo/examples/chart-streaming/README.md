# Streaming chart

Three waveforms scroll by, each the latest 15,000 samples of a signal
sampled 1,000 times a second, with noise and isolated spikes. Every frame
takes in the samples that arrived since the last, keeps the extremes of
each pixel column with `chart.MinMax`, and strokes them with
`Painter.StrokePolyline`.

```sh
go run ./examples/chart-streaming
go run ./examples/chart-streaming -mode path   # MinMax, then StrokePath
go run ./examples/chart-streaming -mode raw    # every sample, StrokePath
go run ./examples/chart-streaming -mode index -samples 1000000
go run ./examples/chart-streaming -mode index -duration 20s
MYGO_FRAME_STATS=all go run ./examples/chart-streaming   # each frame's timing
```

`index` uses `chart.MinMaxIndex` and the same `StrokePolyline` path as
`polyline`. Both use the same deterministic raw samples and viewport.
History is bounded at twice the visible sample count; when it expires,
the example resets the indexes and generates the retained window again.
This deliberately exposes rebuild costs instead of growing history forever.
The window requests 1360×800 DIP; the plot's physical width follows its
layout and the display scale. `MYGO_FRAME_STATS` measures frames including
renderer submission, not GPU execution or physical display scanout.
