package main

import (
	"os"

	"github.com/cangyunye/go-owl/cmd/cli/cmd"
	"github.com/cangyunye/go-owl/internal/encoding"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/locale"
)

func main() {
	loc := locale.Resolve()
	i18n.Init(loc.Lang)
	encoding.Setup()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}