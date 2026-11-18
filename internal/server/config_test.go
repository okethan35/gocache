// internal/server/config_test.go
package server

import (
	"context"
	"testing"
	"time"

	"gocache/internal/store"
)

// SPEC.md Phase 6 rule 9: a negative Config field is an error, returned
// without serving.
func TestServeConfigRejectsNegativeFields(t *testing.T) {
	for name, cfg := range map[string]Config{
		"MaxClients":   {MaxClients: -1},
		"IdleTimeout":  {IdleTimeout: -time.Second},
		"WriteTimeout": {WriteTimeout: -time.Second},
	} {
		ln := listen(t)
		done := make(chan error, 1)
		go func() { done <- ServeConfig(context.Background(), ln, store.New(), cfg) }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: expected an error for a negative value, got nil", name)
			}
		case <-time.After(time.Second):
			t.Errorf("%s: ServeConfig started serving with a negative value", name)
			ln.Close()
			<-done
		}
		ln.Close()
	}
}
