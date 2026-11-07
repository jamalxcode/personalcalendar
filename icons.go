package main

import (
	_ "embed"
	"image/color"
)

// Default highlight colour (royal blue) — used where Go needs a colour.
var accent = color.NRGBA{R: 0x41, G: 0x69, B: 0xE1, A: 0xFF}

// App + menu bar icon: the smiling calendar with the magic wand (assets/).
//
//go:embed assets/appicon.png
var appIcon []byte

//go:embed assets/trayicon.png
var trayIcon []byte

func appIconPNG() []byte  { return appIcon }
func trayIconPNG() []byte { return trayIcon }
