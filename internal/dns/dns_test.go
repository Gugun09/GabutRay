package dns

import (
	"strings"
	"testing"
)

func TestSetupCommands(t *testing.T) {
	commands := SetupCommands("gabutray0", []string{"1.1.1.1", "1.0.0.1"})
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"sudo resolvectl dns gabutray0 1.1.1.1 1.0.0.1",
		"sudo resolvectl domain gabutray0 '~.'",
		"sudo resolvectl default-route gabutray0 yes",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}
