package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gabutray/internal/config"
	"gabutray/internal/dns"
	"gabutray/internal/runtime"
)

func Report(socket string, paths config.Paths, cfg config.Config) string {
	lines := []string{
		"Gabutray doctor",
		checkCommand("ip"),
		checkCommand("systemctl"),
		checkCommand("sudo"),
		checkCommand("resolvectl"),
		checkCommand("xray"),
		checkCommand("tun2socks"),
		"config: " + paths.ConfigFile,
		"config xray_bin: " + checkFileOrPath(cfg.XrayBin),
		"config tun2socks_bin: " + checkFileOrPath(cfg.Tun2socksBin),
		dns.StatusText(cfg.DNSEnabled, cfg.DNSServers),
		fmt.Sprintf("dns strict: %s", enabledText(cfg.DNSStrict)),
	}
	if route, err := runtime.DefaultRouteInfo(); err == nil {
		lines = append(lines, fmt.Sprintf("ok: default route -> dev %s via %s", route.Dev, valueOr(route.Gateway, "<direct>")))
	} else {
		lines = append(lines, "missing: default route")
	}
	if _, err := os.Stat(socket); err == nil {
		lines = append(lines, "ok: daemon socket exists at "+socket)
	} else {
		lines = append(lines, "warn: daemon socket not found at "+socket)
	}
	return strings.Join(lines, "\n")
}

func checkCommand(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "missing: " + name
	}
	return fmt.Sprintf("ok: %s -> %s", name, path)
}

func checkFileOrPath(path string) string {
	if !strings.Contains(path, "/") {
		return path + " (PATH lookup)"
	}
	info, err := os.Stat(path)
	if err != nil {
		return "missing -> " + path
	}
	if info.IsDir() {
		return "not a file -> " + path
	}
	return "ok -> " + path
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
