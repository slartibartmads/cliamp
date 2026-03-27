package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cliamp/playlist"
	"cliamp/theme"
)

func init() {
	RegisterProvisionalPlugin("albumart", func() ProvisionalPlugin { return new(AlbumArt) })
}

// AlbumArt manages cover art fetching, caching, scaling, and rendering.
// It owns all cover-art state so that the Model struct stays clean.
// Zero value is valid; no constructor required.
type AlbumArt struct {
	path     string      // path of track whose image is cached
	key      string      // art source identifier (URL or "embedded:<len>")
	image    image.Image // decoded source image; nil if absent
	scaled   *image.RGBA // rescaled to current display dimensions; nil = needs rescale
	fetching bool        // true while an async HTTP fetch is in-flight
	hidden   bool        // when true, art is not rendered

	// lazy rescale cache — scaled is reused when all three match
	lastArtCols int
	lastHeight  int
	lastMode    CoverArtMode

	Mode CoverArtMode // block character set (sextant/quadrant/half-block/bitmap)
}

// OnTick implements ProvisionalPlugin. Detects track changes and kicks off fetches.
func (a *AlbumArt) OnTick(track playlist.Track) tea.Cmd {

	// Detect track change and clear cached art.
	if track.Path != a.path {
		a.path = track.Path

		newKey := track.CoverArtURL
		if newKey == "" && len(track.CoverArt) > 0 {
			newKey = fmt.Sprintf("embedded:%d", len(track.CoverArt))
		}

		if newKey == "" || newKey != a.key {
			a.key = newKey
			a.image = nil
			a.scaled = nil
			a.fetching = false
			if len(track.CoverArt) > 0 {
				a.image = decodeCoverArt(track.CoverArt)
			}
		}
	}

	// Kick off an async fetch if needed.
	if a.image == nil && !a.fetching && track.CoverArtURL != "" && track.Path == a.path {
		a.fetching = true
		return fetchCoverArtCmd(track.Path, track.CoverArtURL)
	}
	return nil
}

// OnMsg implements ProvisionalPlugin. Handles CoverArtFetchedMsg.
func (a *AlbumArt) OnMsg(msg tea.Msg) tea.Cmd {
	fetched, ok := msg.(CoverArtFetchedMsg)
	if !ok {
		return nil
	}
	if fetched.path == a.path {
		a.image = fetched.img
		a.scaled = nil // force rescale on next Render
		a.fetching = false
	}
	return nil
}

// RenderHeader implements ProvisionalHeaderProvider. Lazily rescales the image when height,
// mode, or panel width has changed, applying the art-derived theme
// as a side effect. Returns ("", 0) when no image is available or hidden,
// restoring the default ANSI theme in that case.
func (a *AlbumArt) RenderHeader(height int) (string, int) {
	if a.image == nil || a.hidden {
		applyTheme(theme.Default())
		return "", 0
	}
	artCols := height * 2
	if artCols > panelWidth/2 {
		artCols = panelWidth / 2
	}
	if a.scaled == nil || artCols != a.lastArtCols || height != a.lastHeight || a.Mode != a.lastMode {
		w, h := coverArtPixelSize(a.Mode, artCols, height)
		a.scaled = scaleImage(a.image, w, h)
		applyTheme(themeFromImage(a.scaled))
		a.lastArtCols = artCols
		a.lastHeight = height
		a.lastMode = a.Mode
	}
	return renderCoverArt(a.scaled, artCols, height, a.Mode), artCols
}

// HelpSuffix implements ProvisionalHelpProvider. Returns the current render mode name
// when art is visible, so the user knows which mode is active.
func (a *AlbumArt) HelpSuffix() string {
	if a.image == nil || a.hidden {
		return ""
	}
	return a.Mode.String()
}

// HandleKey implements ProvisionalKeyHandler. c cycles the render mode, C toggles visibility.
func (a *AlbumArt) HandleKey(key string) (bool, string) {
	switch key {
	case "c":
		a.CycleMode()
		return true, ""
	case "C":
		a.hidden = !a.hidden
		a.scaled = nil
		return true, ""
	}
	return false, ""
}

// OnResize implements ProvisionalPlugin. Invalidates the scaled cache so the
// image is re-rendered at the new terminal dimensions on the next Render.
func (a *AlbumArt) OnResize() {
	a.scaled = nil
}

// CycleMode advances to the next CoverArtMode and returns the new mode name.
func (a *AlbumArt) CycleMode() string {
	a.Mode = (a.Mode + 1) % 4
	a.scaled = nil // force rescale on next Render
	return a.Mode.String()
}

// CoverArtFetchedMsg carries the result of an async cover art HTTP fetch.
type CoverArtFetchedMsg struct {
	path string // track path the fetch was initiated for
	img  image.Image
}

// fetchCoverArtCmd fetches cover art from artURL in a goroutine and returns a
// CoverArtFetchedMsg. path is used to discard stale results if the track changes.
func fetchCoverArtCmd(path, artURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(artURL) //nolint:noctx
		if err != nil {
			return CoverArtFetchedMsg{path: path}
		}
		defer resp.Body.Close()
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return CoverArtFetchedMsg{path: path}
		}
		return CoverArtFetchedMsg{path: path, img: img}
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

// CoverArtMode selects the Unicode block character set used to render album art.
type CoverArtMode int

const (
	CoverArtSextant   CoverArtMode = iota // 2×3 pixels/cell — Unicode 13 sextant blocks (default)
	CoverArtQuadrant                      // 2×2 pixels/cell — Unicode quadrant blocks
	CoverArtHalfBlock                     // 1×2 pixels/cell — Unicode half blocks
	CoverArtBitmap                        // Kitty terminal graphics protocol
)

func (m CoverArtMode) String() string {
	switch m {
	case CoverArtQuadrant:
		return "quadrant"
	case CoverArtHalfBlock:
		return "half-block"
	case CoverArtBitmap:
		return "bitmap"
	default:
		return "sextant"
	}
}

// coverArtPixelSize returns the pixel dimensions the source image must be
// scaled to before rendering with the given mode and cell dimensions.
func coverArtPixelSize(mode CoverArtMode, cols, rows int) (w, h int) {
	switch mode {
	case CoverArtHalfBlock:
		return cols, rows * 2
	case CoverArtQuadrant:
		return cols * 2, rows * 2
	case CoverArtBitmap:
		return 256, 256
	default: // CoverArtSextant
		return cols * 2, rows * 3
	}
}

// quadrantChars maps a 4-bit pixel pattern to a Unicode quadrant block character.
// Bits: 3=top-left, 2=top-right, 1=bottom-left, 0=bottom-right.
var quadrantChars = [16]rune{
	' ', '▗', '▖', '▄', '▝', '▐', '▞', '▟',
	'▘', '▚', '▌', '▙', '▀', '▜', '▛', '█',
}

// sextantChars maps a 6-bit pixel pattern to a Unicode sextant character.
// Bits: 0=top-left, 1=top-right, 2=mid-left, 3=mid-right, 4=bot-left, 5=bot-right.
// Patterns 0/63 use space/█; patterns 21/42 reuse ▌/▐ (skipped in Unicode 13 range).
// The remaining 60 patterns map to U+1FB00–U+1FB3B.
var sextantChars = func() [64]rune {
	var t [64]rune
	t[0] = ' '
	t[21] = '▌' // left column: bits 0+2+4
	t[42] = '▐' // right column: bits 1+3+5
	t[63] = '█'
	for p := 1; p <= 62; p++ {
		if p == 21 || p == 42 {
			continue
		}
		skips := 0
		if p > 21 {
			skips++
		}
		if p > 42 {
			skips++
		}
		t[p] = rune(0x1FB00 + p - 1 - skips)
	}
	return t
}()

// renderCoverArt renders a pre-scaled RGBA image as width×height terminal cells
// using the given mode's block characters and ANSI true-color sequences.
// The image must have been scaled to coverArtPixelSize(mode, width, height).
// Returns an empty string if scaled is nil or dimensions are invalid.
func renderCoverArt(scaled *image.RGBA, width, height int, mode CoverArtMode) string {
	if scaled == nil || width <= 0 || height <= 0 {
		return ""
	}
	// In non-bitmap modes, clear any lingering Kitty image from a previous
	// bitmap render. The delete-all APC is position-independent and harmless
	// when no image is present.
	const kittyDeleteAll = "\x1b_Ga=d,d=A,q=2\x1b\\"

	switch mode {
	case CoverArtHalfBlock:
		return kittyDeleteAll + renderHalfBlockArt(scaled, width, height)
	case CoverArtQuadrant:
		return kittyDeleteAll + renderQuadrantArt(scaled, width, height)
	case CoverArtBitmap:
		return renderBitmapArt(scaled, width, height)
	default:
		return kittyDeleteAll + renderSextantArt(scaled, width, height)
	}
}

// renderHalfBlockArt renders using ▀ with fg=top pixel, bg=bottom pixel per cell.
// Input image must be width × height*2 pixels.
func renderHalfBlockArt(scaled *image.RGBA, width, height int) string {
	rows := make([]string, height)
	for row := range height {
		var sb strings.Builder
		for col := range width {
			r0, g0, b0, _ := scaled.At(col, row*2).RGBA()
			r1, g1, b1, _ := scaled.At(col, row*2+1).RGBA()
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀\x1b[0m",
				r0>>8, g0>>8, b0>>8, r1>>8, g1>>8, b1>>8)
		}
		rows[row] = sb.String()
	}
	return strings.Join(rows, "\n")
}

// renderQuadrantArt renders using Unicode quadrant block characters (2×2 pixels/cell).
// Input image must be width*2 × height*2 pixels.
func renderQuadrantArt(scaled *image.RGBA, width, height int) string {
	rows := make([]string, height)
	for row := range height {
		var sb strings.Builder
		for col := range width {
			var px [4][3]uint32 // [tl, tr, bl, br][r, g, b]
			for i, c := range [4][2]int{
				{col * 2, row * 2}, {col*2 + 1, row * 2},
				{col * 2, row*2 + 1}, {col*2 + 1, row*2 + 1},
			} {
				r, g, b, _ := scaled.At(c[0], c[1]).RGBA()
				px[i] = [3]uint32{r >> 8, g >> 8, b >> 8}
			}
			fgR, fgG, fgB, bgR, bgG, bgB, char := assignFgBg4(px, quadrantChars[:])
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c\x1b[0m",
				fgR, fgG, fgB, bgR, bgG, bgB, char)
		}
		rows[row] = sb.String()
	}
	return strings.Join(rows, "\n")
}

// renderSextantArt renders using Unicode sextant block characters (2×3 pixels/cell).
// Input image must be width*2 × height*3 pixels.
func renderSextantArt(scaled *image.RGBA, width, height int) string {
	rows := make([]string, height)
	for row := range height {
		var sb strings.Builder
		for col := range width {
			var px [6][3]uint32 // [tl, tr, ml, mr, bl, br][r, g, b]
			for i, c := range [6][2]int{
				{col * 2, row * 3}, {col*2 + 1, row * 3},
				{col * 2, row*3 + 1}, {col*2 + 1, row*3 + 1},
				{col * 2, row*3 + 2}, {col*2 + 1, row*3 + 2},
			} {
				r, g, b, _ := scaled.At(c[0], c[1]).RGBA()
				px[i] = [3]uint32{r >> 8, g >> 8, b >> 8}
			}
			fgR, fgG, fgB, bgR, bgG, bgB, char := assignFgBg6(px, sextantChars[:])
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c\x1b[0m",
				fgR, fgG, fgB, bgR, bgG, bgB, char)
		}
		rows[row] = sb.String()
	}
	return strings.Join(rows, "\n")
}

// renderBitmapArt transmits the image using the Kitty terminal graphics protocol
// and returns a single-line string that occupies cols×rows cells in the layout.
// Rows 1+ of the returned slice are empty — the Kitty image persists over them.
// Requires a Kitty-compatible terminal (e.g. kitty, WezTerm, Ghostty).
func renderBitmapArt(scaled *image.RGBA, cols, rows int) string {
	// Encode raw RGBA pixels as base64.
	encoded := base64.StdEncoding.EncodeToString(scaled.Pix)
	b := scaled.Bounds()

	var sb strings.Builder

	// Delete any previously placed image to prevent stacking on redraws.
	sb.WriteString("\x1b_Ga=d,d=A,q=2\x1b\\")

	// Transmit in chunks of ≤4096 base64 bytes as required by the protocol.
	const chunkSize = 4096
	for i := 0; i < len(encoded); i += chunkSize {
		end := min(i+chunkSize, len(encoded))
		more := 1
		if end == len(encoded) {
			more = 0
		}
		if i == 0 {
			// First chunk carries all display parameters:
			//   f=32  → raw RGBA pixels
			//   s,v   → source pixel dimensions
			//   c,r   → display size in terminal cells (Kitty scales to fit)
			//   C=1   → do not move cursor after placing (we advance manually)
			//   q=2   → suppress terminal responses
			fmt.Fprintf(&sb, "\x1b_Ga=T,f=32,s=%d,v=%d,c=%d,r=%d,C=1,q=2,m=%d;%s\x1b\\",
				b.Dx(), b.Dy(), cols, rows, more, encoded[i:end])
		} else {
			fmt.Fprintf(&sb, "\x1b_Gm=%d,q=2;%s\x1b\\", more, encoded[i:end])
		}
	}

	// The Kitty image is an overlay; just move to the next line so the
	// surrounding layout stays flush at the left margin.
	sb.WriteByte('\n')

	// Return as the first of `rows` lines; the rest are empty so Bubbletea does
	// not write over the image area on subsequent rows.
	lines := make([]string, rows)
	lines[0] = sb.String()
	return strings.Join(lines, "\n")
}

// assignFgBg4 assigns 4 pixels to fg/bg by luma, averages each group's color,
// and returns the fg/bg RGB values and the character from chars[pattern].
// Bit i set means pixel i is foreground; bit ordering is MSB-first (bit 3-i).
func assignFgBg4(px [4][3]uint32, chars []rune) (fgR, fgG, fgB, bgR, bgG, bgB uint32, ch rune) {
	var meanLuma float64
	for _, p := range px {
		meanLuma += luma(p)
	}
	meanLuma /= 4
	pattern := 0
	for i, p := range px {
		if luma(p) >= meanLuma {
			pattern |= 1 << (3 - i)
		}
	}
	fgR, fgG, fgB, bgR, bgG, bgB = groupColors4(px, pattern)
	return fgR, fgG, fgB, bgR, bgG, bgB, chars[pattern]
}

// assignFgBg6 assigns 6 pixels to fg/bg by luma, averages each group's color,
// and returns the fg/bg RGB values and the character from chars[pattern].
// Bit i set means pixel i is foreground.
func assignFgBg6(px [6][3]uint32, chars []rune) (fgR, fgG, fgB, bgR, bgG, bgB uint32, ch rune) {
	var meanLuma float64
	for _, p := range px {
		meanLuma += luma(p)
	}
	meanLuma /= 6
	pattern := 0
	for i, p := range px {
		if luma(p) >= meanLuma {
			pattern |= 1 << i
		}
	}
	fgR, fgG, fgB, bgR, bgG, bgB = groupColors6(px, pattern)
	return fgR, fgG, fgB, bgR, bgG, bgB, chars[pattern]
}

func luma(p [3]uint32) float64 {
	return 0.2126*float64(p[0]) + 0.7152*float64(p[1]) + 0.0722*float64(p[2])
}

func groupColors4(px [4][3]uint32, pattern int) (fgR, fgG, fgB, bgR, bgG, bgB uint32) {
	var fgN, bgN uint32
	for i, p := range px {
		if (pattern>>(3-i))&1 == 1 {
			fgR += p[0]
			fgG += p[1]
			fgB += p[2]
			fgN++
		} else {
			bgR += p[0]
			bgG += p[1]
			bgB += p[2]
			bgN++
		}
	}
	if fgN > 0 {
		fgR /= fgN
		fgG /= fgN
		fgB /= fgN
	}
	if bgN > 0 {
		bgR /= bgN
		bgG /= bgN
		bgB /= bgN
	}
	if fgN == 0 {
		fgR, fgG, fgB = bgR, bgG, bgB
	}
	if bgN == 0 {
		bgR, bgG, bgB = fgR, fgG, fgB
	}
	return
}

func groupColors6(px [6][3]uint32, pattern int) (fgR, fgG, fgB, bgR, bgG, bgB uint32) {
	var fgN, bgN uint32
	for i, p := range px {
		if (pattern>>i)&1 == 1 {
			fgR += p[0]
			fgG += p[1]
			fgB += p[2]
			fgN++
		} else {
			bgR += p[0]
			bgG += p[1]
			bgB += p[2]
			bgN++
		}
	}
	if fgN > 0 {
		fgR /= fgN
		fgG /= fgN
		fgB /= fgN
	}
	if bgN > 0 {
		bgR /= bgN
		bgG /= bgN
		bgB /= bgN
	}
	if fgN == 0 {
		fgR, fgG, fgB = bgR, bgG, bgB
	}
	if bgN == 0 {
		bgR, bgG, bgB = fgR, fgG, fgB
	}
	return
}

// scaleImage scales src to width×height using bicubic interpolation.
func scaleImage(src image.Image, width, height int) *image.RGBA {
	return scaleImageSeparable(src, width, height, kernelBicubic, 2)
}

func kernelBicubic(x float64) float64 {
	const a = -0.5
	if x < 0 {
		x = -x
	}
	if x < 1 {
		return (a+2)*x*x*x - (a+3)*x*x + 1
	}
	if x < 2 {
		return a*x*x*x - 5*a*x*x + 8*a*x - 4*a
	}
	return 0
}

func clampU8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// scaleImageSeparable scales src to width×height with a separable kernel filter
// in two passes (horizontal then vertical). radius is the kernel support radius.
func scaleImageSeparable(src image.Image, width, height int, kernel func(float64) float64, radius int) *image.RGBA {
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(src.Bounds())
		draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	}
	sb := rgba.Bounds()
	sw, sh := sb.Dx(), sb.Dy()

	// Horizontal pass: sw×sh → width×sh
	tmp := image.NewRGBA(image.Rect(0, 0, width, sh))
	for y := range sh {
		for x := range width {
			srcX := (float64(x)+0.5)*float64(sw)/float64(width) - 0.5
			var r, g, b, wsum float64
			for k := int(srcX) - radius + 1; k <= int(srcX)+radius; k++ {
				kk := max(0, min(sw-1, k))
				w := kernel(srcX - float64(k))
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
			tmp.SetRGBA(x, y, color.RGBA{R: clampU8(r), G: clampU8(g), B: clampU8(b), A: 255})
		}
	}

	// Vertical pass: width×sh → width×height
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			srcY := (float64(y)+0.5)*float64(sh)/float64(height) - 0.5
			var r, g, b, wsum float64
			for k := int(srcY) - radius + 1; k <= int(srcY)+radius; k++ {
				kk := max(0, min(sh-1, k))
				w := kernel(srcY - float64(k))
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
			dst.SetRGBA(x, y, color.RGBA{R: clampU8(r), G: clampU8(g), B: clampU8(b), A: 255})
		}
	}
	return dst
}
