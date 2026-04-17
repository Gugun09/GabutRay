package xray

import (
	"testing"

	"gabutray/internal/profile"
)

func TestBuildVLESSRealityOutbound(t *testing.T) {
	p, err := profile.ParseShareLink("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=reality&type=tcp&sni=www.example.com&fp=chrome&pbk=abc&sid=def#main")
	if err != nil {
		t.Fatal(err)
	}
	out, err := BuildOutbound(p)
	if err != nil {
		t.Fatal(err)
	}
	if out["protocol"] != "vless" {
		t.Fatalf("unexpected outbound: %+v", out)
	}
	stream := out["streamSettings"].(map[string]any)
	if stream["security"] != "reality" {
		t.Fatalf("unexpected stream: %+v", stream)
	}
}

func TestBuildClientConfigWithSocksInbound(t *testing.T) {
	p, err := profile.ParseShareLink("trojan://secret@example.com:443?security=tls&type=tcp&sni=example.com#trojan")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := BuildClientConfig(p, 10808)
	if err != nil {
		t.Fatal(err)
	}
	inbounds := cfg["inbounds"].([]any)
	first := inbounds[0].(map[string]any)
	if first["protocol"] != "socks" {
		t.Fatalf("unexpected inbound: %+v", first)
	}
}
