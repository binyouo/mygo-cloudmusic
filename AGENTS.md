# AGENTS.md

## Tests

Only write a test for behaviour that can actually break. If the code under test could change without the test noticing, or the test would fail on a harmless rewording, don't write it.

**Write tests for:**

- What input does: clicks, keys, drags, gestures and scrolling, and the state or effect they lead to (seeking when the progress bar is let go, Space after opening Now Playing, back and forward, flinging the cover, liking a song, leaving a radio).
- Logic that doesn't need a view: parsing lyrics, formatting times and counts, the player's queue, seeking, fading and cache, decoding streams and images.
- A regression you just fixed. Say in the test's comment what went wrong.
- Frame budgets, in `perf_test.go`, so idle views don't start asking for frames they don't need.

**Don't write tests that:**

- Check that a list of labels shows on screen. That repeats the view code and breaks whenever the wording changes.
- Check default settings or the order of rows in a form.
- Test MyGo itself, for example that `AnimateTo` or a spring moves from 0 to 1.
- Depend on the wall clock. `ui.NewTester` starts its clock at the current time, so anything with a phase taken from the clock passes or fails depending on when the test runs. Also, `FramePending` ignores frames waiting on `After` or `Painter.After`.

**How:**

- Build views with `ui.NewTester` and set up the app with `newTestApp` (in `internal/app/helpers_test.go`). Find elements by their accessibility label, then `Click`, `RightClick`, `Press`/`Move`/`Release` or `Key`, and check the app's state afterwards.
- Swap out the network and the clipboard through package variables such as `likeSong` and `copyText`, and restore the real one with `defer`.
- Live tests that call NetEase skip unless `LIVE` is set.
- Look at visuals instead of asserting on them. The `NM_FRAMES` tests save PNGs into that directory and skip when it isn't set. Check pixels only for coverage, such as whether a layer still covers the window.
- Start each test with a one-sentence comment saying the behaviour it checks, in the style of the code.
- Run `go test ./internal/...` before committing, and run a new UI test with `-count=5`. Fix or delete a flaky test; don't retry it.

## iOS checkout

- The desktop build uses `third_party/mygo` through `go.mod` and carries a local fix for pointer hover during momentum scrolling. iOS builds use the module pinned in `go.ios.mod` through `scripts/mygo-ios.sh`. Keep the MyGo versions aligned and run iOS tests with `GOFLAGS=-modfile=go.ios.mod go test ./internal/...`.

### iOS device delivery and acceptance

- After each change to the iOS app, build the current checkout with the existing iPhone development signing and install it on the user's paired iPhone over Wi-Fi. Use the existing development bundle ID (`dev.egoist.cloudmusic.dev`) and a build number higher than the one on the phone. Keep device signing overrides in a separate build directory.
- Check that `xcrun devicectl list devices` reports the iPhone's transport as `localNetwork` before installing with `xcrun devicectl device install app`.
- Read back the installed bundle ID, version, and build number with `xcrun devicectl device info apps`. Report the installed build and what to try on the phone. If wireless installation or readback is blocked, report the blocker and which step succeeded.
- The user performs hands-on acceptance on their iPhone, including visuals, animations, gestures, and playback. Leave the app ready for them to open and wait for their acceptance. Automated tests remain available for development checks.
