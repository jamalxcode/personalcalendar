package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

var accent = color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0xFF}

// trayIconPNG draws a calendar glyph as a macOS template image (black + alpha),
// so the menu bar recolours it for light/dark mode. 36px = 18pt @2x.
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
	fill(4, 7, 32, 13)  // header
	fill(4, 7, 7, 32)   // left
	fill(29, 7, 32, 32) // right
	fill(4, 29, 32, 32) // bottom
	fill(10, 3, 13, 10) // ring left
	fill(23, 3, 26, 10) // ring right
	for _, p := range [][2]int{{10, 17}, {16, 17}, {22, 17}, {10, 23}, {16, 23}} {
		fill(p[0], p[1], p[0]+4, p[1]+3)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// appIconPNG: red-topped rounded calendar used for the .app bundle icon.
func appIconPNG() []byte {
	const S = 1024
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	white := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	dot := color.NRGBA{0x3A, 0x3A, 0x3C, 0xFF}
	x0, y0, x1, y1, r := 100, 100, 924, 924, 185 // macOS icon grid: 824px body
	in := func(x, y int) bool {
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			return false
		}
		cx, cy := x, y
		if x < x0+r {
			cx = x0 + r
		} else if x >= x1-r {
			cx = x1 - r - 1
		}
		if y < y0+r {
			cy = y0 + r
		} else if y >= y1-r {
			cy = y1 - r - 1
		}
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			if in(x, y) {
				if y < 330 {
					img.Set(x, y, accent)
				} else {
					img.Set(x, y, white)
				}
			}
		}
	}
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			bx, by := 215+col*160, 410+row*140
			c := dot
			if row == 1 && col == 2 {
				c = accent
			}
			for y := by; y < by+80; y++ {
				for x := bx; x < bx+100; x++ {
					img.Set(x, y, c)
				}
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
