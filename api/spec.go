package api

import (
	"embed"
	_ "embed"
)

//go:embed *.yaml
var Spec embed.FS
