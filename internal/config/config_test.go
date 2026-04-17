package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		Root:            root,
		ConfigFile:      filepath.Join(root, "config.yaml"),
		ProfilesFile:    filepath.Join(root, "profiles.yaml"),
		GeneratedConfig: filepath.Join(root, "xray.generated.json"),
		RuntimeFile:     filepath.Join(root, "runtime.json"),
		LogDir:          filepath.Join(root, "logs"),
	}

	cfg, err := Load(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SocksPort != 10808 || cfg.TunName != "gabutray0" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.DNSEnabled || !cfg.DNSStrict || len(cfg.DNSServers) != 2 || cfg.DNSServers[0] != "1.1.1.1" {
		t.Fatalf("unexpected DNS defaults: %+v", cfg)
	}
}

func TestSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	paths := buildPaths(root, "")
	want := Config{
		XrayBin:      "/tmp/xray",
		Tun2socksBin: "/tmp/tun2socks",
		SocksPort:    18080,
		TunName:      "testtun0",
		TunCIDR:      "198.19.0.1/16",
		DNSEnabled:   true,
		DNSServers:   []string{"9.9.9.9"},
		DNSStrict:    false,
	}

	if err := Save(paths, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseDNSServers(t *testing.T) {
	got, err := ParseDNSServers("1.1.1.1, 1.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "1.1.1.1" || got[1] != "1.0.0.1" {
		t.Fatalf("unexpected servers: %#v", got)
	}
	if _, err := ParseDNSServers("not-an-ip"); err == nil {
		t.Fatal("expected invalid IP error")
	}
	if _, err := ParseDNSServers("2606:4700:4700::1111"); err == nil {
		t.Fatal("expected IPv6 DNS server to be rejected")
	}
}
