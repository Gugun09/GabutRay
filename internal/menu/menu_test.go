package menu

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gabutray/internal/config"
	"gabutray/internal/latency"
	"gabutray/internal/profile"
	"gabutray/internal/runtime"
)

func TestViewHomeShowsActiveProfileAndLatency(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{
		Root:            root,
		ConfigFile:      filepath.Join(root, "config.yaml"),
		ProfilesFile:    filepath.Join(root, "profiles.yaml"),
		GeneratedConfig: filepath.Join(root, "xray.generated.json"),
		RuntimeFile:     filepath.Join(root, "runtime.json"),
		LogDir:          filepath.Join(root, "logs"),
	}
	item := profile.Profile{
		ID:       "main",
		Name:     "main",
		Protocol: profile.ProtocolVLESS,
		Address:  "example.com",
		Port:     443,
	}
	m := newModel(Options{}, paths, config.DefaultConfig())
	m.profiles = []profile.Profile{item}
	m.active = &runtime.State{ProfileID: "main", ProfileName: "main", TunName: "gabutray0", TunCIDR: "198.18.0.1/15"}
	m.latencyResults = map[string]latency.Result{
		"main": {
			Profile:  item,
			Address:  "example.com:443",
			Status:   latency.StatusOK,
			Duration: 48 * time.Millisecond,
		},
	}

	out := m.viewHome()
	for _, want := range []string{"Status", "Terhubung", "main", "example.com:443", "48 ms", "*"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
