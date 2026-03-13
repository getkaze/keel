package main

import "embed"

//go:embed all:web
var embeddedFS embed.FS
