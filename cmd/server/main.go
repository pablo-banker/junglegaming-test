package main

import (
	"time"

	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/wiring"
)

// stopTimeout bounds graceful shutdown; work still running after it is rolled back.
const stopTimeout = 20 * time.Second

func main() {
	fx.New(
		wiring.App,
		fx.StopTimeout(stopTimeout),
	).Run()
}
