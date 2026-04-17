package profile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func ParseShareLink(link string) (Profile, error) {
	switch {
	case strings.HasPrefix(link, "vless://"):
		return parseVLESS(link)
	case strings.HasPrefix(link, "vmess://"):
		return parseVMess(link)
	case strings.HasPrefix(link, "trojan://"):
		return parseTrojan(link)
	default:
		return Profile{}, errors.New("unsupported share link; expected vless://, vmess://, or trojan://")
	}
}

func parseVLESS(link string) (Profile, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid VLESS share link: %w", err)
	}
	address, err := requiredHost(u)
	if err != nil {
		return Profile{}, err
	}
	port, err := requiredPort(u, "VLESS")
	if err != nil {
		return Profile{}, err
	}
	id := u.User.Username()
	if id == "" {
		return Profile{}, errors.New("VLESS link is missing UUID")
	}
	query := queryMap(u)
	encryption := query["encryption"]
	if encryption == "" {
		encryption = "none"
	}
	return Profile{
		Name:          linkName(u, address, ProtocolVLESS),
		Protocol:      ProtocolVLESS,
		Address:       address,
		Port:          port,
		Auth:          Auth{ID: id, Encryption: encryption, Flow: query["flow"]},
		Security:      securityFromQuery(query, "none"),
		Transport:     transportFromQuery(query),
		RawLink:       link,
		CreatedAtUnix: time.Now().Unix(),
	}, nil
}

func parseTrojan(link string) (Profile, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid Trojan share link: %w", err)
	}
	address, err := requiredHost(u)
	if err != nil {
		return Profile{}, err
	}
	port, err := requiredPort(u, "Trojan")
	if err != nil {
		return Profile{}, err
	}
	password := u.User.Username()
	if password == "" {
		return Profile{}, errors.New("Trojan link is missing password")
	}
	query := queryMap(u)
	return Profile{
		Name:          linkName(u, address, ProtocolTrojan),
		Protocol:      ProtocolTrojan,
		Address:       address,
		Port:          port,
		Auth:          Auth{Password: password},
		Security:      securityFromQuery(query, "tls"),
		Transport:     transportFromQuery(query),
		RawLink:       link,
		CreatedAtUnix: time.Now().Unix(),
	}, nil
}

func parseVMess(link string) (Profile, error) {
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := decodeBase64(strings.TrimSpace(encoded))
	if err != nil {
		return Profile{}, fmt.Errorf("VMess link is not valid base64: %w", err)
	}
	var share vmessShare
	if err := json.Unmarshal(decoded, &share); err != nil {
		return Profile{}, fmt.Errorf("VMess payload is not valid JSON: %w", err)
	}
	address, err := requiredString(share.Add, "VMess add")
	if err != nil {
		return Profile{}, err
	}
	portText, err := requiredString(share.Port, "VMess port")
	if err != nil {
		return Profile{}, err
	}
	port, err := parsePort(portText, "VMess port")
	if err != nil {
		return Profile{}, err
	}
	id, err := requiredString(share.ID, "VMess id")
	if err != nil {
		return Profile{}, err
	}
	name := strings.TrimSpace(share.PS)
	if name == "" {
		name = defaultName(ProtocolVMess, address)
	}
	network := share.Net
	if network == "" {
		network = "tcp"
	}
	securityKind := share.TLS
	if securityKind == "" {
		securityKind = "none"
	}
	security := NoneSecurity()
	security.Kind = securityKind
	security.SNI = share.SNI
	security.Fingerprint = share.FP
	security.ALPN = splitCSV(share.ALPN)
	alterID := 0
	if share.AID != "" {
		alterID, err = parseUint16(share.AID, "VMess alterId")
		if err != nil {
			return Profile{}, err
		}
	}
	return Profile{
		Name:     name,
		Protocol: ProtocolVMess,
		Address:  address,
		Port:     port,
		Auth: Auth{
			ID:       id,
			AlterID:  alterID,
			Security: share.SCY,
		},
		Security: security,
		Transport: TransportSettings{
			Network:    network,
			Host:       share.Host,
			Path:       share.Path,
			HeaderType: share.Type,
		},
		RawLink:       link,
		CreatedAtUnix: time.Now().Unix(),
	}, nil
}

func decodeBase64(value string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var last error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		last = err
	}
	return nil, last
}

func queryMap(u *url.URL) map[string]string {
	out := make(map[string]string)
	for key, values := range u.Query() {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func securityFromQuery(query map[string]string, defaultKind string) SecuritySettings {
	kind := query["security"]
	if kind == "" {
		kind = defaultKind
	}
	return SecuritySettings{
		Kind:          kind,
		SNI:           first(query, "sni", "peer", "serverName"),
		Fingerprint:   first(query, "fp", "fingerprint"),
		PublicKey:     first(query, "pbk", "publicKey"),
		ShortID:       first(query, "sid", "shortId"),
		SpiderX:       first(query, "spx", "spiderX"),
		AllowInsecure: boolish(first(query, "allowInsecure", "allow_insecure")),
		ALPN:          splitCSV(query["alpn"]),
	}
}

func transportFromQuery(query map[string]string) TransportSettings {
	network := first(query, "type", "network")
	if network == "" {
		network = "tcp"
	}
	return TransportSettings{
		Network:     network,
		Host:        first(query, "host", "authority"),
		Path:        query["path"],
		ServiceName: first(query, "serviceName", "service_name"),
		Mode:        query["mode"],
		HeaderType:  first(query, "headerType", "header_type"),
	}
}

func first(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return ""
}

func boolish(value string) bool {
	return value == "1" || strings.EqualFold(value, "true")
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func requiredHost(u *url.URL) (string, error) {
	if u.Hostname() == "" {
		return "", errors.New("share link is missing address")
	}
	return u.Hostname(), nil
}

func requiredPort(u *url.URL, protocol string) (int, error) {
	portText := u.Port()
	if portText == "" {
		return 0, fmt.Errorf("%s link is missing port", protocol)
	}
	return parsePort(portText, protocol+" port")
}

func parsePort(value, field string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid %s: %s", field, value)
	}
	return port, nil
}

func parseUint16(value, field string) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 || number > 65535 {
		return 0, fmt.Errorf("invalid %s: %s", field, value)
	}
	return number, nil
}

func requiredString(value, field string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is missing", field)
	}
	return value, nil
}

func linkName(u *url.URL, address string, protocol Protocol) string {
	if fragment := strings.TrimSpace(u.Fragment); fragment != "" {
		return fragment
	}
	return defaultName(protocol, address)
}

func defaultName(protocol Protocol, address string) string {
	return fmt.Sprintf("%s-%s", protocol, address)
}

type vmessShare struct {
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	AID  string `json:"aid"`
	SCY  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
	FP   string `json:"fp"`
}
