package hue

import (
	"math"
)

// ToXyY converts a 24-bit RGB value into a value in the CIE xyY color space.
// Based on https://github.com/PhilipsHue/PhilipsHueSDK-iOS-OSX/blob/master/ApplicationDesignNotes/RGB%20to%20xy%20Color%20conversion.md
func ToXyY(r float64, g float64, b float64) (float64, float64) {
	// scale
	r, g, b = r/255.0, g/255.0, b/255.0

	// gamma correct
	if r > 0.04045 {
		r = math.Pow((r+0.055)/(1.0+0.055), 2.4)
	} else {
		r /= 12.92
	}
	if g > 0.04045 {
		g = math.Pow((g+0.055)/(1.0+0.055), 2.4)
	} else {
		g /= 12.92
	}
	if b > 0.04045 {
		b = math.Pow((b+0.055)/(1.0+0.055), 2.4)
	} else {
		b /= 12.92
	}

	// convert to XYZ in wide RGB D65 space
	X := r*0.649926 + g*0.103455 + b*0.197109
	Y := r*0.234327 + g*0.743075 + b*0.022598
	Z := r*0.000000 + g*0.053077 + b*1.035763

	x := X / (X + Y + Z)
	if math.IsNaN(x) {
		x = 0.0
	}
	y := Y / (X + Y + Z)
	if math.IsNaN(y) {
		y = 0.0
	}

	// check if (x, y) is contained within the triangle
	if !inLampsReach(x, y) {
		x, y = closestPointOnTriangle(x, y)
	}

	return x, y
}

// point is an x-y coordinate.
type point struct {
	x float64
	y float64
}

// Color point constants for the Hue color space
var colorPoints = []point{
	point{0.675, 0.322},  // red
	point{0.4091, 0.518}, // green
	point{0.167, 0.04}}   // blue

// Color point indexes
const (
	CPT_RED   = 0
	CPT_GREEN = 1
	CPT_BLUE  = 2
)

// crossProduct calculates the cross product of two vectors.
func crossProduct(p1 point, p2 point) float64 {
	return p1.x*p2.y - p1.y*p2.x
}

// inLampsReach returns true if the given point is in the lamp's color space
// (assuming a Hue bulb).
func inLampsReach(x, y float64) bool {
	red := colorPoints[CPT_RED]
	green := colorPoints[CPT_GREEN]
	blue := colorPoints[CPT_BLUE]

	v1 := point{green.x - red.x, green.y - red.y}
	v2 := point{blue.x - red.x, blue.y - red.y}
	q := point{x - red.x, y - red.y}

	s := crossProduct(q, v2) / crossProduct(v1, v2)
	t := crossProduct(v1, q) / crossProduct(v1, v2)

	return s >= 0.0 && t >= 0.0 && s+t <= 1.0
}

// getClosestPointToPoints gets the point on line (p1, p2) closest to p3.
func closestPointOnLine(a point, b point, p point) point {
	ap := point{p.x - a.x, p.y - a.y}
	ab := point{b.x - a.x, b.y - a.y}
	ab2 := ab.x*ab.x + ab.y*ab.y
	ap_ab := ap.x*ab.x + ap.y*ab.y
	t := ap_ab / ab2

	if t < 0.0 {
		t = 0.0
	} else if t > 1.0 {
		t = 1.0
	}

	return point{a.x + ab.x*t, a.y + ab.y*t}
}

// distance returns the magnitude of the distance from point a to point b.
func distance(a point, b point) float64 {
	dx := a.x - b.x
	dy := a.y - b.y
	return math.Sqrt(dx*dx + dy*dy)
}

// closestPointOnTriangle returns the closest point on the color triangle to a
// given point p.
func closestPointOnTriangle(x, y float64) (float64, float64) {
	p := point{x, y}

	// find the closest color producible with the Hue
	pAB := closestPointOnLine(colorPoints[CPT_RED], colorPoints[CPT_GREEN], p)
	pAC := closestPointOnLine(colorPoints[CPT_BLUE], colorPoints[CPT_RED], p)
	pBC := closestPointOnLine(colorPoints[CPT_GREEN], colorPoints[CPT_BLUE], p)

	dAB := distance(p, pAB)
	dAC := distance(p, pAC)
	dBC := distance(p, pBC)

	switch math.Min(math.Min(dAB, dAC), dBC) {
	case dAB:
		return pAB.x, pAB.y
	case dAC:
		return pAC.x, pAC.y
	default:
		return pBC.x, pBC.y
	}
}
