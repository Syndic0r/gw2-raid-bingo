package discord

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestRecoverGuardSwallowsPanic proves a panic inside a bot goroutine is turned
// into a logged error instead of crashing the process (which would take down the
// co-hosted web server too).
func TestRecoverGuardSwallowsPanic(t *testing.T) {
	var buf bytes.Buffer
	b := &Bot{log: log.New(&buf, "", 0)}

	func() {
		defer b.recoverGuard("unit test")
		panic("boom")
	}()

	got := buf.String()
	if !strings.Contains(got, "PANIC in unit test") {
		t.Fatalf("expected panic to be logged, got %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("expected the panic value in the log, got %q", got)
	}
}
