//go:build production || debug

package main

import "embed"

// The production build runs after Wails/Vite has generated frontend/dist.
// Debug builds also embed the generated frontend so `wails build -debug`
// produces a self-contained window.
//
//go:embed all:frontend/dist
var assets embed.FS
