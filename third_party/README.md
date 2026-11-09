# Local MyGo patch

`mygo/` is the Go module snapshot of `github.com/egoist/mygo` at
`v0.7.5-0.20261106165234-3d46dbb2ad93` (commit `3d46dbb2ad93`). Its upstream
license is retained in `mygo/LICENSE`. The root `go.mod` replaces the module
with this snapshot so builds and tests use the same input fix.

Local changes:

- `ui/input.go`: momentum scroll events keep their event location for scroll
  routing without overwriting the pointer's current position or window presence.
- `ui/handler.go`: scroll handlers receive coordinates and deltas relative to
  the scroll event's location, independently of the pointer's current position.
- `ui/momentum_hover_regression_test.go`: ordinary and virtual lists keep
  scrolling while the pointer moves to the footer, another pane, or outside the
  window, without restoring stale row hover. Fresh wheel and touchpad input
  still update hover normally.

This fixes stale highlights in both the song list and sidebar playlists. When
upstream includes the fix, remove the replacement and this snapshot together.
