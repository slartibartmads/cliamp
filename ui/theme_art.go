package ui

import (
	"fmt"
	"image"
	"math"

	"cliamp/theme"
)

// themeFromImage derives a UI Theme from the most vibrant color in the
// already-scaled cover art image. Returns theme.Default() for desaturated
// (near-greyscale) images.
func themeFromImage(img *image.RGBA) theme.Theme {
	if img == nil {
		return theme.Default()
	}
	b := img.Bounds()

	var bestH, bestS, bestV float64
	bestScore := -1.0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			h, s, v := rgbToHSV(float64(c.R), float64(c.G), float64(c.B))
			// Reward high saturation and mid-range brightness (not near black/white).
			score := s * s * (1 - math.Abs(v-0.55)*1.5)
			if score > bestScore {
				bestScore = score
				bestH, bestS, bestV = h, s, v
			}
		}
	}

	// Fall back to default for greyscale / near-monochrome artwork.
	if bestScore < 0.08 {
		return theme.Default()
	}

	s := math.Max(bestS, 0.5)
	v := math.Max(math.Min(bestV, 0.80), 0.70) // clamp: readable on black, not blinding
	accent := hsvToHex(bestH, s, v)

	// Brighter variant: same hue and saturation, boosted value.
	bright := hsvToHex(bestH, s*0.85, math.Min(v*1.35, 1.0))

	// Spectrum: monochromatic ramp from dim → base → bright.
	specLow := hsvToHex(bestH, s*0.7, v*0.65)

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

// restoreBaseTheme re-applies whatever theme was selected before art overrode it.
func (m *Model) restoreBaseTheme() {
	if m.themeIdx < 0 {
		applyTheme(theme.Default())
	} else {
		applyTheme(m.themes[m.themeIdx])
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
