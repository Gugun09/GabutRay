package profile

import (
	"encoding/base64"
	"testing"
)

func TestParseVLESSReality(t *testing.T) {
	link := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=reality&type=tcp&sni=www.example.com&fp=chrome&pbk=abc&sid=def&flow=xtls-rprx-vision#main"
	p, err := ParseShareLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != ProtocolVLESS || p.Name != "main" || p.Security.Kind != "reality" {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if p.Security.PublicKey != "abc" || p.Auth.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected reality/auth fields: %+v", p)
	}
}

func TestParseVLESSWSTLS(t *testing.T) {
	link := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=ws&host=edge.example.com&path=%2Fws&sni=edge.example.com#ws"
	p, err := ParseShareLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Transport.Network != "ws" || p.Transport.Path != "/ws" || p.Transport.Host != "edge.example.com" {
		t.Fatalf("unexpected transport: %+v", p.Transport)
	}
}

func TestParseTrojan(t *testing.T) {
	p, err := ParseShareLink("trojan://secret@example.com:443?security=tls&type=tcp&sni=example.com#trojan")
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != ProtocolTrojan || p.Auth.Password != "secret" || p.Security.Kind != "tls" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestParseVMessBase64JSON(t *testing.T) {
	payload := `{"v":"2","ps":"vm","add":"example.com","port":"443","id":"11111111-1111-1111-1111-111111111111","aid":"0","scy":"auto","net":"ws","type":"none","host":"example.com","path":"/ray","tls":"tls","sni":"example.com","fp":"chrome"}`
	link := "vmess://" + base64.RawStdEncoding.EncodeToString([]byte(payload))
	p, err := ParseShareLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != ProtocolVMess || p.Name != "vm" || p.Transport.Network != "ws" || p.Security.Kind != "tls" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("Main Server"); got != "main-server" {
		t.Fatalf("got %q", got)
	}
	if got := Slugify(" !!! "); got != "profile" {
		t.Fatalf("got %q", got)
	}
}
