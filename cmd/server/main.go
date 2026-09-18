package main

import (
	"github.com/pablo-banker/junglegaming-test/internal/wiring"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		wiring.App,
	).Run()
}
