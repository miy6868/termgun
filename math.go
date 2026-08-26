package main

import "math"

// A terminal cell is roughly twice as tall as it is wide. All aiming, ranges
// and speeds are computed in "visual space", where a vertical world unit counts
// double, so circles look like circles and bullets fly at an even pace in every
// direction.
const aspect = 2.0

type Vec struct{ X, Y float64 }

func (v Vec) Add(o Vec) Vec       { return Vec{v.X + o.X, v.Y + o.Y} }
func (v Vec) Sub(o Vec) Vec       { return Vec{v.X - o.X, v.Y - o.Y} }
func (v Vec) Scale(s float64) Vec { return Vec{v.X * s, v.Y * s} }

// visual converts world coordinates into aspect-corrected space.
func (v Vec) visual() Vec { return Vec{v.X, v.Y * aspect} }

// unvisual converts an aspect-corrected vector back into world space.
func (v Vec) unvisual() Vec { return Vec{v.X, v.Y / aspect} }

// len is the plain square root rather than math.Hypot. Hypot exists to survive
// intermediate overflow and underflow when squaring, which costs it a scaling
// pass and several branches — and it showed up in the frame profile, because
// every range check, aim and knockback goes through here. Coordinates in this
// game are level tiles: a few hundred at the very most, so the squares cannot
// leave the comfortable middle of the float64 range.
func (v Vec) len() float64 { return math.Sqrt(v.X*v.X + v.Y*v.Y) }

func (v Vec) norm() Vec {
	l := v.len()
	if l < 1e-9 {
		return Vec{}
	}
	return Vec{v.X / l, v.Y / l}
}

func (v Vec) rotate(rad float64) Vec {
	s, c := math.Sincos(rad)
	return Vec{v.X*c - v.Y*s, v.X*s + v.Y*c}
}

// vdist is the distance between two world points as it appears on screen.
func vdist(a, b Vec) float64 { return a.Sub(b).visual().len() }

// aimDir returns a unit vector (in visual space) pointing from a to b.
func aimDir(from, to Vec) Vec { return to.Sub(from).visual().norm() }

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
