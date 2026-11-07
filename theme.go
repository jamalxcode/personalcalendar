package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var accent = color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0xFF} // Fantastical-ish red

type calTheme struct{}

func (calTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return accent
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0x40}
	case theme.ColorNameBackground:
		if v == theme.VariantDark {
			return color.NRGBA{R: 0x1C, G: 0x1C, B: 0x1E, A: 0xFF}
		}
		return color.NRGBA{R: 0xF7, G: 0xF7, B: 0xF8, A: 0xFF}
	case theme.ColorNameInputBackground:
		if v == theme.VariantDark {
			return color.NRGBA{R: 0x2C, G: 0x2C, B: 0x2E, A: 0xFF}
		}
		return color.White
	}
	return theme.DefaultTheme().Color(n, v)
}
func (calTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (calTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (calTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameInnerPadding:
		return 7
	}
	return theme.DefaultTheme().Size(n)
}

// cardBG / mutedText adapt to light & dark.
func cardBG() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		return color.NRGBA{R: 0x2A, G: 0x2A, B: 0x2D, A: 0xFF}
	}
	return color.White
}
func heroBG() color.Color {
	return color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0x22}
}
func mutedText() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		return color.NRGBA{R: 0x9A, G: 0x9A, B: 0xA0, A: 0xFF}
	}
	return color.NRGBA{R: 0x6E, G: 0x6E, B: 0x73, A: 0xFF}
}

func hexColor(s string) color.Color {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return accent
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return accent
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

// trayIconPNG draws a small calendar glyph as a macOS template image (black + alpha).
func trayIconPNG() []byte {
	const S = 36
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	on := color.NRGBA{A: 0xFF}
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.Set(x, y, on)
			}
		}
	}
	// body outline
	fill(4, 7, 32, 10)  // top bar (thick header)
	fill(4, 10, 32, 13) // header
	fill(4, 7, 7, 32)   // left
	fill(29, 7, 32, 32) // right
	fill(4, 29, 32, 32) // bottom
	fill(10, 3, 13, 10) // ring left
	fill(23, 3, 26, 10) // ring right
	// dots grid
	for _, p := range [][2]int{{10, 17}, {16, 17}, {22, 17}, {10, 23}, {16, 23}} {
		fill(p[0], p[1], p[0]+4, p[1]+3)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// appIcon: red rounded calendar for the window / Dock.
func appIcon() fyne.Resource {
	const S = 256
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	white := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	r := 48
	inRounded := func(x, y, x0, y0, x1, y1, rad int) bool {
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			return false
		}
		cx, cy := x, y
		if x < x0+rad {
			cx = x0 + rad
		} else if x >= x1-rad {
			cx = x1 - rad - 1
		}
		if y < y0+rad {
			cy = y0 + rad
		} else if y >= y1-rad {
			cy = y1 - rad - 1
		}
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= rad*rad
	}
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			if inRounded(x, y, 16, 16, 240, 240, r) {
				if y < 84 {
					img.Set(x, y, accent)
				} else {
					img.Set(x, y, white)
				}
			}
		}
	}
	// simple grid of dots on the white part
	dot := color.NRGBA{0x3A, 0x3A, 0x3C, 0xFF}
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			x0, y0 := 50+col*44, 110+row*40
			for y := y0; y < y0+20; y++ {
				for x := x0; x < x0+24; x++ {
					c := dot
					if row == 1 && col == 2 {
						c = accent
					}
					img.Set(x, y, c)
				}
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return fyne.NewStaticResource("icon.png", b.Bytes())
}
