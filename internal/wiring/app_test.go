package wiring

import (
	"testing"

	"go.uber.org/fx"
)

// TestAppGraphIsComplete verifies every constructor dependency of the application is provided.
func TestAppGraphIsComplete(t *testing.T) {
	if err := fx.ValidateApp(App); err != nil {
		t.Fatalf("invalid Fx graph: %v", err)
	}
}
