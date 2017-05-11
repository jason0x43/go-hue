package hue

import (
	"log"
	"math"
)

// Gamut is a color gamut
type Gamut struct {
	red   point
	green point
	blue  point
}

// GetGamut gets the color gamut for a particular bulb model
func GetGamut(model string) Gamut {
	switch model {
	case "LST001", "LLC010", "LLC011", "LLC012", "LLC006", "LLC007", "LLC013":
		return gamutA
	case "LCT001", "LCT007", "LCT002", "LCT003", "LLM001":
		return gamutB
	case "LCT010", "LCT014", "LCT011", "LLC020", "LST002":
		return gamutC
	default:
		return gamutD
	}
}

// ToXyY converts a 24-bit RGB value into a value in the CIE xyY color space.
// Based on https://github.com/PhilipsHue/PhilipsHueSDK-iOS-OSX/blob/master/ApplicationDesignNotes/RGB%20to%20xy%20Color%20conversion.md
func (gamut *Gamut) ToXyY(r, g, b int) (x, y, Y float64) {
	// scale down
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	// gamma correct
	if rf > 0.04045 {
		rf = math.Pow((rf+0.055)/(1.0+0.055), 2.4)
	} else {
		rf /= 12.92
	}
	if gf > 0.04045 {
		gf = math.Pow((gf+0.055)/(1.0+0.055), 2.4)
	} else {
		gf /= 12.92
	}
	if bf > 0.04045 {
		bf = math.Pow((bf+0.055)/(1.0+0.055), 2.4)
	} else {
		bf /= 12.92
	}

	// convert to XYZ in wide RGB D65 space
	X := rf*0.664511 + gf*0.154324 + bf*0.162028
	Y = rf*0.283881 + gf*0.668433 + bf*0.047685
	Z := rf*0.000088 + gf*0.072310 + bf*0.986039

	x = X / (X + Y + Z)
	if math.IsNaN(x) {
		x = 0.0
	}
	y = Y / (X + Y + Z)
	if math.IsNaN(y) {
		y = 0.0
	}

	// check if (x, y) is contained within the triangle
	if !gamut.inLampsReach(x, y) {
		log.Printf("Not in reach")
		x, y = gamut.closestPointOnTriangle(x, y)
	}

	return
}

// ToRGB converts an XY value in the CIE into a 24-bit RGB value.
func (gamut *Gamut) ToRGB(x, y, bri float64) (r, g, b uint8) {
	// check if (x, y) is contained within the triangle
	if !gamut.inLampsReach(x, y) {
		log.Printf("Not in reach")
		x, y = gamut.closestPointOnTriangle(x, y)
	}

	z := 1.0 - x - y
	Y := bri
	X := (Y / y) * x
	Z := (Y / y) * z

	// Convert to RGB using Wide RGB D65 conversion
	rf := X*1.656492 - Y*0.354851 - Z*0.255038
	gf := -X*0.707196 + Y*1.655397 + Z*0.036152
	bf := X*0.051713 - Y*0.121364 + Z*1.011530

	// Reverse gamma correction
	if rf <= 0.0031308 {
		rf *= 12.92
	} else {
		rf = (1.0+0.055)*math.Pow(rf, (1.0/2.4)) - 0.055
	}
	if gf <= 0.0031308 {
		gf *= 12.92
	} else {
		gf = (1.0+0.055)*math.Pow(gf, (1.0/2.4)) - 0.055
	}
	if bf <= 0.0031308 {
		bf *= 12.92
	} else {
		bf = (1.0+0.055)*math.Pow(bf, (1.0/2.4)) - 0.055
	}

	// Normalize around largest value if a value is > 1
	max := math.Max(math.Max(rf, gf), bf)
	if max > 1 {
		rf, gf, bf = rf/max, gf/max, bf/max
	}

	// Scale up
	r = uint8(math.Ceil(rf*255.0 - 0.5))
	g = uint8(math.Ceil(gf*255.0 - 0.5))
	b = uint8(math.Ceil(bf*255.0 - 0.5))

	return
}

// ToHSL converts an XY value in the CIE into an HSL value.
func (gamut *Gamut) ToHSL(x, y, bri float64) (h, s, l float64) {
	r, g, b := gamut.ToRGB(x, y, bri)
	return rgbToHsl(r, g, b)
}

// Convert an RGB value to an HSL value, where H is in [0, 360], S is in [0, 1], and L is in [0, 1]
func rgbToHsl(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	max := math.Max(math.Max(rf, gf), bf)
	min := math.Min(math.Min(rf, gf), bf)

	l = (max + min) / 2.0

	if max == min {
		// achromatic
		h = 0
		s = 0
	} else {
		d := max - min
		if l > 0.5 {
			s = d / (2.0 - max - min)
		} else {
			s = d / (max + min)
		}

		switch max {
		case rf:
			h = (gf - bf) / d
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/d + 2.0
		case bf:
			h = (rf-gf)/d + 4.0
		}

		h /= 6.0
	}

	h *= 360.0

	return
}

// inLampsReach returns true if the given point is in the lamp's color space
// (assuming a Hue bulb).
func (gamut *Gamut) inLampsReach(x, y float64) bool {
	red := gamut.red
	green := gamut.green
	blue := gamut.blue

	v1 := point{green.x - red.x, green.y - red.y}
	v2 := point{blue.x - red.x, blue.y - red.y}
	q := point{x - red.x, y - red.y}

	s := crossProduct(q, v2) / crossProduct(v1, v2)
	t := crossProduct(v1, q) / crossProduct(v1, v2)

	return s >= 0.0 && t >= 0.0 && s+t <= 1.0
}

// closestPointOnTriangle returns the closest point on the color triangle to a
// given point p.
func (gamut *Gamut) closestPointOnTriangle(x, y float64) (float64, float64) {
	p := point{x, y}

	// find the closest color producible with the Hue
	pAB := closestPointOnLine(gamut.red, gamut.green, p)
	pAC := closestPointOnLine(gamut.blue, gamut.red, p)
	pBC := closestPointOnLine(gamut.green, gamut.blue, p)

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

// point is an x-y coordinate.
type point struct {
	x float64
	y float64
}

// LivingColors Iris, Bloom, Aura, LightStrips
var gamutA = Gamut{
	red:   point{0.704, 0.296},   // red
	green: point{0.2151, 0.7106}, // green
	blue:  point{0.138, 0.08},    // blue
}

// Original Hue bulbs
var gamutB = Gamut{
	red:   point{0.675, 0.322},  // red
	green: point{0.4091, 0.518}, // green
	blue:  point{0.167, 0.04},   // blue
}

// Hue Gen 3, Hue Go, LightStrips plus, BR30
var gamutC = Gamut{
	red:   point{0.692, 0.308}, // red
	green: point{0.17, 0.7},    // green
	blue:  point{0.153, 0.048}, // blue
}

// Default space
var gamutD = Gamut{
	red:   point{1.0, 0.0}, // red
	green: point{0.0, 1.0}, // green
	blue:  point{0.0, 0.0}, // blue
}

// crossProduct calculates the cross product of two vectors.
func crossProduct(p1 point, p2 point) float64 {
	return p1.x*p2.y - p1.y*p2.x
}

// getClosestPointToPoints gets the point on line (p1, p2) closest to p3.
func closestPointOnLine(a point, b point, p point) point {
	ap := point{p.x - a.x, p.y - a.y}
	ab := point{b.x - a.x, b.y - a.y}
	ab2 := ab.x*ab.x + ab.y*ab.y
	apAb := ap.x*ab.x + ap.y*ab.y
	t := apAb / ab2

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
