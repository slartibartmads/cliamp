package ui

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"cliamp/theme"
)

const topNColors = 50 // number of top-scoring pixels to average

// colorSample holds a scored HSV color for averaging.
type colorSample struct {
	h, s, v float64
	score   float64
}

// themeFromImage derives a UI Theme from the dominant color in the
// already-scaled cover art image.  Averages the top-N most vibrant pixels
// for a stable accent.  Returns theme.Default() for desaturated images.
func themeFromImage(img *image.RGBA) theme.Theme {
	h, s, v, _ := extractHSV(img)
	if s < 0.01 {
		return theme.Default()
	}
	return themeFromHSV(h, s, v)
}

// extractHSV returns the averaged HSV of the top-N scoring pixels in img.
// score is the average score of those pixels (0 for greyscale images).
func extractHSV(img image.Image) (h, s, v, score float64) {
	if img == nil {
		return 0, 0, 0, 0
	}
	// Convert to RGBA if needed for pixel access.
	rgba, ok := img.(*image.RGBA)
	var b image.Rectangle
	if !ok {
		rgba = image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		b = rgba.Bounds()
	} else {
		b = rgba.Bounds()
	}

	minScore := 0.0

	// Collect top-N pixels by score.
	var pool [topNColors]colorSample

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := rgba.RGBAAt(x, y)
			ph, ps, pv := rgbToHSV(float64(c.R), float64(c.G), float64(c.B))
			sc := ps * ps * (1 - math.Abs(pv-0.55)*1.5)
			if sc <= 0 {
				continue
			}
			// Replace the weakest entry if this pixel scores higher.
			if sc > minScore || pool[0].score == 0 {
				// Find the min-scoring slot.
				worst := 0
				for i := 1; i < topNColors; i++ {
					if pool[i].score < pool[worst].score {
						worst = i
					}
				}
				pool[worst] = colorSample{ph, ps, pv, sc}
				// Recompute minScore.
				minScore = pool[0].score
				for i := 1; i < topNColors; i++ {
					if pool[i].score < minScore {
						minScore = pool[i].score
					}
				}
			}
		}
	}

	// Average the collected samples.
	// Hue is circular — average via sin/cos to handle wrap-around.
	var sumSin, sumCos, sumS, sumV, sumScore float64
	var n float64
	for _, s := range pool {
		if s.score <= 0 {
			continue
		}
		rad := s.h * math.Pi / 180
		sumSin += math.Sin(rad) * s.score
		sumCos += math.Cos(rad) * s.score
		sumS += s.s * s.score
		sumV += s.v * s.score
		sumScore += s.score
		n += s.score
	}

	if n == 0 {
		return 0, 0, 0, 0
	}

	h = math.Atan2(sumSin, sumCos) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	s = sumS / n
	v = sumV / n
	score = sumScore / topNColors
	return
}

// themeFromHSV builds a Theme from a single HSV accent color.
func themeFromHSV(h, s, v float64) theme.Theme {
	s = math.Max(s, 0.5)
	v = math.Max(math.Min(v, 0.80), 0.70)

	accent := hsvToHex(h, s, v)
	bright := hsvToHex(h, s*0.85, math.Min(v*1.35, 1.0))
	specLow := hsvToHex(h, s*0.7, v*0.65)

	return theme.Theme{
		Name:     "Album Art",
		Accent:   bright,
		BrightFG: "#e8e8e8",
		FG:       "#999999",
		Green:    specLow,
		Yellow:   accent,
		Red:      bright,
	}
}

func rgbToHSV(r, g, b float64) (h, s, v float64) {
	r, g, b = r/255, g/255, b/255
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	d := mx - mn
	v = mx
	if mx == 0 {
		return 0, 0, v
	}
	s = d / mx
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6) * 60
	case g:
		h = ((b-r)/d + 2) * 60
	default:
		h = ((r-g)/d + 4) * 60
	}
	if h < 0 {
		h += 360
	}
	return
}

func hsvToHex(h, s, v float64) string {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h/60) % 6 {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return fmt.Sprintf("#%02x%02x%02x",
		int((r+m)*255+0.5), int((g+m)*255+0.5), int((b+m)*255+0.5))
}
