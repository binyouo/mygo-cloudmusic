package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// Momentum used to restore row hover at the gesture origin after the pointer moved to the footer, another pane, or outside the window.
func TestMomentumDoesNotMovePointerBack(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		t.Run(fmt.Sprint("virtual=", virtual), func(t *testing.T) {
			var state ScrollState
			var listState ListState
			var hovered bool
			var otherHovered bool
			var got InputEvent
			tt := NewTester(func(c *Context) {
				hovered = false
				Row(c).Fill().Children(func() {
					Column(c).Width(200).Children(func() {
						row := func(i int) {
							b := ButtonBase(c.Key(i)).Height(30).Label(fmt.Sprint("Row ", i))
							hovered = b.Hovered() || hovered
						}
						if virtual {
							List(c, &listState, 100, row).Height(200).HandleInput(func(ev InputEvent) bool {
								if ev.Kind == InputScroll {
									got = ev
								}
								return false
							})
						} else {
							Scroll(c).Height(200).TrackScroll(&state).HandleInput(func(ev InputEvent) bool {
								if ev.Kind == InputScroll {
									got = ev
								}
								return false
							}).Children(func() {
								for i := 0; i < 100; i++ {
									row(i)
								}
							})
						}
						Box(c).Height(100).Label("Footer")
					})
					other := Scroll(c).Width(100).Height(300)
					otherHovered = other.Hovered()
					other.Children(func() { Box(c).Height(1000) })
				})
			}, 300, 300)
			tt.Frame()
			tt.Move(50, 50)
			tt.Scroll(50, 50, 0, 30)
			if !hovered {
				t.Fatal("the row under the pointer is not hovered")
			}
			tt.Move(50, 250)
			if hovered {
				t.Fatal("moving into the footer did not clear hover")
			}
			before := float64(state.Y)
			if virtual {
				before = listState.wroteY
			}
			tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: 50, Y: 50, DY: 60, Precise: true, Momentum: true})
			if hovered {
				t.Error("momentum moved hover back into the list after the pointer left it")
			}
			if got.X != 50 || got.Y != 50 || !got.Momentum {
				t.Errorf("scroll handler got %+v", got)
			}
			after := float64(state.Y)
			if virtual {
				after = listState.wroteY
			}
			if after <= before {
				t.Errorf("momentum stopped scrolling the original pane: %v -> %v", before, after)
			}
			tt.Move(250, 250)
			tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: 50, Y: 50, DY: 30, Precise: true, Momentum: true})
			if hovered || !otherHovered {
				t.Error("momentum moved hover out of the other pane")
			}
			tt.send(platform.SurfaceEvent{Kind: platform.PointerLeave})
			tt.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: 50, Y: 50, DY: 30, Precise: true, Momentum: true})
			if hovered || tt.rt.pointerIn {
				t.Error("momentum brought the pointer back into the window")
			}
			// Fresh touchpad and mouse-wheel gestures still locate the pointer;
			// a wheel's remaining glide must not restore hover after it leaves.
			for _, wheel := range []bool{false, true} {
				if wheel {
					tt.Wheel(50, 50, 0, 30)
				} else {
					tt.Scroll(50, 50, 0, 30)
				}
				if !hovered {
					t.Fatalf("fresh scroll did not update hover (wheel=%v)", wheel)
				}
				tt.Move(50, 250)
				tt.Advance(time.Second)
				if hovered {
					t.Fatalf("scroll glide restored hover after leaving (wheel=%v)", wheel)
				}
			}
		})
	}
}
