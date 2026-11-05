//go:build !production && !debug

package main

import "embed"

// Development and test builds do not embed frontend/dist. `wails dev` serves
// the React application from Vite, and Go tests should not require a frontend
// build just to compile the Wails entry point.
var assets embed.FS
