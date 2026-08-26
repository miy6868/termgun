package main

// Zoom is how many terminal cells one world tile occupies, on both axes.
//
// It has to be the same number for X and Y. The simulation already treats a
// vertical world unit as counting double (see aspect in math.go), which is what
// makes a tile appear square in a terminal whose cells are twice as tall as they
// are wide. Scaling the axes by different amounts would undo that and turn every
// circle in the game into an ellipse.
const (
	minZoom     = 1
	maxZoom     = 4
	defaultZoom = 2
)

// A zoomed-in view shows fewer tiles, and past some point the player cannot see
// a threat until it is already on top of them. These floors cap the zoom that a
// given terminal size is allowed to use, whatever the player asked for.
const (
	minTilesW = 24
	minTilesH = 8
)

// fitZoom reduces want until the playfield still shows enough of the dungeon.
// The requested level is left untouched, so widening the terminal restores it.
func fitZoom(want, viewW, viewH int) int {
	z := clamp(want, minZoom, maxZoom)
	for z > minZoom && (viewW/z < minTilesW || viewH/z < minTilesH) {
		z--
	}
	return z
}
