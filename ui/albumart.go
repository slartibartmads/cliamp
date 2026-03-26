package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// coverArtFetchedMsg carries the result of an async cover art HTTP fetch.
type coverArtFetchedMsg struct {
	path string // track path the fetch was initiated for
	img  image.Image
}

// fetchCoverArtCmd fetches cover art from artURL in a goroutine and returns a
// coverArtFetchedMsg. path is used to discard stale results if the track changes.
func fetchCoverArtCmd(path, artURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(artURL) //nolint:noctx
		if err != nil {
			return coverArtFetchedMsg{path: path}
		}
		defer resp.Body.Close()
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return coverArtFetchedMsg{path: path}
		}
		return coverArtFetchedMsg{path: path, img: img}
	}
}

// decodeCoverArt decodes raw image bytes into an image.Image.
// Returns nil if data is empty or cannot be decoded.
func decodeCoverArt(data []byte) image.Image {
	if len(data) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

// quadrantChars maps a 4-bit pixel pattern to a Unicode block character.
// Bits: 3=top-left, 2=top-right, 1=bottom-left, 0=bottom-right.
// A set bit means the pixel belongs to the foreground color.
var quadrantChars = [16]rune{
	' ', '▗', '▖', '▄', '▝', '▐', '▞', '▟',
	'▘', '▚', '▌', '▙', '▀', '▜', '▛', '█',
}

// renderCoverArt renders a pre-scaled RGBA image (width*2 × height*2 pixels)
// as width×height terminal cells using Unicode quadrant block characters with
// ANSI true-color sequences. Each cell encodes a 2×2 pixel block.
// Returns an empty string if scaled is nil or dimensions are invalid.
func renderCoverArt(scaled *image.RGBA, width, height int) string {
	if scaled == nil || width <= 0 || height <= 0 {
		return ""
	}

	rows := make([]string, height)
	for row := range height {
		var sb strings.Builder
		for col := range width {
			// Sample the 2×2 pixel block for this cell.
			var px [4][3]uint32 // [tl, tr, bl, br][r, g, b]
			for i, c := range [4][2]int{
				{col*2, row*2}, {col*2 + 1, row*2},
				{col*2, row*2 + 1}, {col*2 + 1, row*2 + 1},
			} {
				r, g, b, _ := scaled.At(c[0], c[1]).RGBA()
				px[i] = [3]uint32{r >> 8, g >> 8, b >> 8}
			}

			// Assign each pixel to fg (1) or bg (0) by comparing to mean luma.
			var meanLuma float64
			for _, p := range px {
				meanLuma += 0.2126*float64(p[0]) + 0.7152*float64(p[1]) + 0.0722*float64(p[2])
			}
			meanLuma /= 4

			pattern := 0
			for i, p := range px {
				luma := 0.2126*float64(p[0]) + 0.7152*float64(p[1]) + 0.0722*float64(p[2])
				if luma >= meanLuma {
					pattern |= 1 << (3 - i)
				}
			}

			// Average the colors within each group.
			var fgR, fgG, fgB, fgN uint32
			var bgR, bgG, bgB, bgN uint32
			for i, p := range px {
				if (pattern>>(3-i))&1 == 1 {
					fgR += p[0]; fgG += p[1]; fgB += p[2]; fgN++
				} else {
					bgR += p[0]; bgG += p[1]; bgB += p[2]; bgN++
				}
			}
			if fgN > 0 {
				fgR /= fgN; fgG /= fgN; fgB /= fgN
			}
			if bgN > 0 {
				bgR /= bgN; bgG /= bgN; bgB /= bgN
			}
			// Degenerate cases: copy the defined group to the undefined one.
			if fgN == 0 {
				fgR, fgG, fgB = bgR, bgG, bgB
			}
			if bgN == 0 {
				bgR, bgG, bgB = fgR, fgG, fgB
			}

			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c\x1b[0m",
				fgR, fgG, fgB, bgR, bgG, bgB, quadrantChars[pattern])
		}
		rows[row] = sb.String()
	}

	return strings.Join(rows, "\n")
}

// scaleImageLanczos scales src to width×height using a separable Lanczos3
// filter (two passes: horizontal then vertical).
func scaleImageLanczos(src image.Image, width, height int) *image.RGBA {
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(src.Bounds())
		draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	}
	sb := rgba.Bounds()
	sw, sh := sb.Dx(), sb.Dy()

	const a = 3 // Lanczos3 radius

	lanczos := func(x float64) float64 {
		if x < 0 {
			x = -x
		}
		if x == 0 {
			return 1
		}
		if x >= a {
			return 0
		}
		px := math.Pi * x
		return float64(a) * math.Sin(px) * math.Sin(px/float64(a)) / (px * px)
	}

	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}

	// Horizontal pass: sw×sh → width×sh
	tmp := image.NewRGBA(image.Rect(0, 0, width, sh))
	for y := range sh {
		for x := range width {
			srcX := (float64(x)+0.5)*float64(sw)/float64(width) - 0.5
			var r, g, b, wsum float64
			for k := int(srcX) - a + 1; k <= int(srcX)+a; k++ {
				kk := k
				if kk < 0 {
					kk = 0
				} else if kk >= sw {
					kk = sw - 1
				}
				w := lanczos(srcX - float64(k))
				c := rgba.RGBAAt(sb.Min.X+kk, sb.Min.Y+y)
				r += w * float64(c.R)
				g += w * float64(c.G)
				b += w * float64(c.B)
				wsum += w
			}
			if wsum > 0 {
				r /= wsum
				g /= wsum
				b /= wsum
			}
			tmp.SetRGBA(x, y, color.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 255})
		}
	}

	// Vertical pass: width×sh → width×height
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			srcY := (float64(y)+0.5)*float64(sh)/float64(height) - 0.5
			var r, g, b, wsum float64
			for k := int(srcY) - a + 1; k <= int(srcY)+a; k++ {
				kk := k
				if kk < 0 {
					kk = 0
				} else if kk >= sh {
					kk = sh - 1
				}
				w := lanczos(srcY - float64(k))
				c := tmp.RGBAAt(x, kk)
				r += w * float64(c.R)
				g += w * float64(c.G)
				b += w * float64(c.B)
				wsum += w
			}
			if wsum > 0 {
				r /= wsum
				g /= wsum
				b /= wsum
			}
			dst.SetRGBA(x, y, color.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 255})
		}
	}
	return dst
}
