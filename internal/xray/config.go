package xray

import (
	"encoding/json"
	"fmt"

	"gabutray/internal/profile"
)

func BuildClientConfig(p profile.Profile, socksPort int) (map[string]any, error) {
	outbound, err := BuildOutbound(p)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "socks-in",
				"protocol": "socks",
				"listen":   "127.0.0.1",
				"port":     socksPort,
				"settings": map[string]any{"udp": true},
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []any{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": []any{
			outbound,
			map[string]any{"tag": "direct", "protocol": "freedom"},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"domainStrategy": "IPIfNonMatch",
			"rules": []any{
				map[string]any{"ip": []any{"geoip:private"}, "outboundTag": "direct"},
				map[string]any{"protocol": []any{"bittorrent"}, "outboundTag": "block"},
			},
		},
	}, nil
}

func BuildOutbound(p profile.Profile) (map[string]any, error) {
	var (
		out map[string]any
		err error
	)
	switch p.Protocol {
	case profile.ProtocolVLESS:
		out, err = buildVLESSOutbound(p)
	case profile.ProtocolVMess:
		out, err = buildVMessOutbound(p)
	case profile.ProtocolTrojan:
		out, err = buildTrojanOutbound(p)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}
	if err != nil {
		return nil, err
	}
	stream, err := buildStreamSettings(p.Security, p.Transport)
	if err != nil {
		return nil, err
	}
	out["streamSettings"] = stream
	return out, nil
}

func MarshalPretty(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func buildVLESSOutbound(p profile.Profile) (map[string]any, error) {
	if p.Auth.ID == "" {
		return nil, fmt.Errorf("VLESS profile missing UUID")
	}
	user := map[string]any{
		"id":         p.Auth.ID,
		"encryption": valueOr(p.Auth.Encryption, "none"),
		"level":      0,
	}
	if p.Auth.Flow != "" {
		user["flow"] = p.Auth.Flow
	}
	return map[string]any{
		"tag":      "proxy",
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []any{
				map[string]any{
					"address": p.Address,
					"port":    p.Port,
					"users":   []any{user},
				},
			},
		},
	}, nil
}

func buildVMessOutbound(p profile.Profile) (map[string]any, error) {
	if p.Auth.ID == "" {
		return nil, fmt.Errorf("VMess profile missing UUID")
	}
	return map[string]any{
		"tag":      "proxy",
		"protocol": "vmess",
		"settings": map[string]any{
			"vnext": []any{
				map[string]any{
					"address": p.Address,
					"port":    p.Port,
					"users": []any{
						map[string]any{
							"id":       p.Auth.ID,
							"alterId":  p.Auth.AlterID,
							"security": valueOr(p.Auth.Security, "auto"),
							"level":    0,
						},
					},
				},
			},
		},
	}, nil
}

func buildTrojanOutbound(p profile.Profile) (map[string]any, error) {
	if p.Auth.Password == "" {
		return nil, fmt.Errorf("Trojan profile missing password")
	}
	return map[string]any{
		"tag":      "proxy",
		"protocol": "trojan",
		"settings": map[string]any{
			"servers": []any{
				map[string]any{
					"address":  p.Address,
					"port":     p.Port,
					"password": p.Auth.Password,
					"level":    0,
				},
			},
		},
	}, nil
}

func buildStreamSettings(security profile.SecuritySettings, transport profile.TransportSettings) (map[string]any, error) {
	stream := map[string]any{
		"network":  transport.Network,
		"security": security.Kind,
	}
	switch security.Kind {
	case "", "none":
		stream["security"] = "none"
	case "tls":
		stream["tlsSettings"] = buildTLSSettings(security)
	case "reality":
		settings, err := buildRealitySettings(security)
		if err != nil {
			return nil, err
		}
		stream["realitySettings"] = settings
	default:
		return nil, fmt.Errorf("unsupported security mode: %s", security.Kind)
	}

	switch transport.Network {
	case "", "tcp":
		stream["network"] = "tcp"
		if transport.HeaderType != "" && transport.HeaderType != "none" {
			stream["tcpSettings"] = map[string]any{"header": map[string]any{"type": transport.HeaderType}}
		}
	case "ws", "websocket":
		stream["network"] = "ws"
		settings := map[string]any{}
		if transport.Path != "" {
			settings["path"] = transport.Path
		}
		if transport.Host != "" {
			settings["headers"] = map[string]any{"Host": transport.Host}
		}
		stream["wsSettings"] = settings
	case "grpc":
		settings := map[string]any{}
		if transport.ServiceName != "" {
			settings["serviceName"] = transport.ServiceName
		}
		if transport.Mode == "multi" {
			settings["multiMode"] = true
		}
		stream["grpcSettings"] = settings
	case "httpupgrade":
		settings := map[string]any{}
		if transport.Path != "" {
			settings["path"] = transport.Path
		}
		if transport.Host != "" {
			settings["host"] = transport.Host
		}
		stream["httpupgradeSettings"] = settings
	case "h2", "http":
		stream["network"] = "http"
		settings := map[string]any{}
		if transport.Path != "" {
			settings["path"] = []any{transport.Path}
		}
		if transport.Host != "" {
			settings["host"] = []any{transport.Host}
		}
		stream["httpSettings"] = settings
	default:
		return nil, fmt.Errorf("unsupported transport network: %s", transport.Network)
	}
	return stream, nil
}

func buildTLSSettings(security profile.SecuritySettings) map[string]any {
	settings := map[string]any{"allowInsecure": security.AllowInsecure}
	if security.SNI != "" {
		settings["serverName"] = security.SNI
	}
	if security.Fingerprint != "" {
		settings["fingerprint"] = security.Fingerprint
	}
	if len(security.ALPN) > 0 {
		settings["alpn"] = security.ALPN
	}
	return settings
}

func buildRealitySettings(security profile.SecuritySettings) (map[string]any, error) {
	if security.PublicKey == "" {
		return nil, fmt.Errorf("REALITY profile missing public key (pbk)")
	}
	settings := map[string]any{"publicKey": security.PublicKey}
	if security.SNI != "" {
		settings["serverName"] = security.SNI
	}
	if security.Fingerprint != "" {
		settings["fingerprint"] = security.Fingerprint
	}
	if security.ShortID != "" {
		settings["shortId"] = security.ShortID
	}
	if security.SpiderX != "" {
		settings["spiderX"] = security.SpiderX
	}
	return settings, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
