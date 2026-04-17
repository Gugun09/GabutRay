package service

import (
	"strings"
	"testing"
)

func TestUnitContainsDaemonCommand(t *testing.T) {
	unit := Unit("/opt/gabutray/gabutray", "/run/gabutrayd.sock")
	if !strings.Contains(unit, "daemon") || !strings.Contains(unit, "/run/gabutrayd.sock") {
		t.Fatalf("unexpected unit:\n%s", unit)
	}
}
