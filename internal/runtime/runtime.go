package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gabutray/internal/config"
	"gabutray/internal/dns"
	"gabutray/internal/profile"
	"gabutray/internal/xray"
)

type ConnectOptions struct {
	XrayBin      string   `json:"xray_bin" yaml:"xray_bin"`
	Tun2socksBin string   `json:"tun2socks_bin" yaml:"tun2socks_bin"`
	SocksPort    int      `json:"socks_port" yaml:"socks_port"`
	TunName      string   `json:"tun_name" yaml:"tun_name"`
	TunCIDR      string   `json:"tun_cidr" yaml:"tun_cidr"`
	DNSEnabled   bool     `json:"dns_enabled" yaml:"dns_enabled"`
	DNSServers   []string `json:"dns_servers" yaml:"dns_servers"`
	DNSStrict    bool     `json:"dns_strict" yaml:"dns_strict"`
	DryRun       bool     `json:"dry_run" yaml:"dry_run"`
}

type State struct {
	ProfileID    string       `json:"profile_id"`
	ProfileName  string       `json:"profile_name"`
	XrayPID      int          `json:"xray_pid"`
	Tun2socksPID int          `json:"tun2socks_pid,omitempty"`
	TunName      string       `json:"tun_name"`
	TunCIDR      string       `json:"tun_cidr"`
	SocksPort    int          `json:"socks_port"`
	ServerRoute  *ServerRoute `json:"server_route,omitempty"`
	DNS          *dns.State   `json:"dns,omitempty"`
}

type ServerRoute struct {
	IP      string `json:"ip"`
	Gateway string `json:"gateway,omitempty"`
	Dev     string `json:"dev"`
}

type DefaultRoute struct {
	Gateway string
	Dev     string
}

func OptionsFromConfig(cfg config.Config, dryRun bool) ConnectOptions {
	return ConnectOptions{
		XrayBin:      cfg.XrayBin,
		Tun2socksBin: cfg.Tun2socksBin,
		SocksPort:    cfg.SocksPort,
		TunName:      cfg.TunName,
		TunCIDR:      cfg.TunCIDR,
		DNSEnabled:   cfg.DNSEnabled,
		DNSServers:   append([]string(nil), cfg.DNSServers...),
		DNSStrict:    cfg.DNSStrict,
		DryRun:       dryRun,
	}
}

func Connect(paths config.Paths, p profile.Profile, opts ConnectOptions, out io.Writer) error {
	if err := config.Ensure(paths); err != nil {
		return err
	}
	if running, err := activeRuntime(paths.RuntimeFile); err == nil && running {
		return errors.New("another profile is active; disconnect first or use --force")
	}

	clientConfig, err := xray.BuildClientConfig(p, opts.SocksPort)
	if err != nil {
		return err
	}
	data, err := xray.MarshalPretty(clientConfig)
	if err != nil {
		return err
	}
	if err := os.WriteFile(paths.GeneratedConfig, data, 0o644); err != nil {
		return fmt.Errorf("write generated Xray config %s: %w", paths.GeneratedConfig, err)
	}

	serverRoute := resolveServerRoute(p.Address)
	if opts.DryRun {
		printConnectPlan(out, paths, p, opts, serverRoute)
		return nil
	}

	xrayLog, xrayErr, err := logFiles(paths.LogDir, "xray.log")
	if err != nil {
		return err
	}
	defer xrayLog.Close()
	defer xrayErr.Close()
	xrayCmd := exec.Command(opts.XrayBin, "-c", paths.GeneratedConfig)
	xrayCmd.Stdin = nil
	xrayCmd.Stdout = xrayLog
	xrayCmd.Stderr = xrayErr
	xrayCmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(opts.XrayBin))
	if err := xrayCmd.Start(); err != nil {
		return fmt.Errorf("start xray %s: %w", opts.XrayBin, err)
	}

	time.Sleep(500 * time.Millisecond)
	if err := setupTUNAndRoutes(opts, serverRoute); err != nil {
		cleanupPartialConnect(opts, xrayCmd.Process.Pid, 0, serverRoute, false)
		return err
	}

	tunLog, tunErr, err := logFiles(paths.LogDir, "tun2socks.log")
	if err != nil {
		cleanupPartialConnect(opts, xrayCmd.Process.Pid, 0, serverRoute, false)
		return err
	}
	defer tunLog.Close()
	defer tunErr.Close()
	tunCmd := privilegedCommand(opts.Tun2socksBin,
		"-device", "tun://"+opts.TunName,
		"-proxy", fmt.Sprintf("socks5://127.0.0.1:%d", opts.SocksPort),
	)
	tunCmd.Stdin = os.Stdin
	tunCmd.Stdout = tunLog
	tunCmd.Stderr = tunErr
	if err := tunCmd.Start(); err != nil {
		cleanupPartialConnect(opts, xrayCmd.Process.Pid, 0, serverRoute, false)
		return fmt.Errorf("start tun2socks %s: %w", opts.Tun2socksBin, err)
	}

	var dnsState *dns.State
	if opts.DNSEnabled {
		state, err := dns.Setup(opts.TunName, opts.DNSServers)
		if err != nil {
			if opts.DNSStrict {
				cleanupPartialConnect(opts, xrayCmd.Process.Pid, tunCmd.Process.Pid, serverRoute, true)
				return fmt.Errorf("setup DNS anti-leak: %w", err)
			}
			fmt.Fprintf(out, "warning: DNS anti-leak not configured: %v\n", err)
		} else {
			dnsState = &state
		}
	}

	state := State{
		ProfileID:    p.ID,
		ProfileName:  p.Name,
		XrayPID:      xrayCmd.Process.Pid,
		Tun2socksPID: tunCmd.Process.Pid,
		TunName:      opts.TunName,
		TunCIDR:      opts.TunCIDR,
		SocksPort:    opts.SocksPort,
		ServerRoute:  serverRoute,
		DNS:          dnsState,
	}
	if err := SaveState(paths.RuntimeFile, state); err != nil {
		cleanupPartialConnect(opts, xrayCmd.Process.Pid, tunCmd.Process.Pid, serverRoute, dnsState != nil)
		return err
	}
	fmt.Fprintf(out, "connected: %s\n", state.ProfileName)
	fmt.Fprintf(out, "xray pid: %d\n", state.XrayPID)
	fmt.Fprintf(out, "tun2socks pid: %d\n", state.Tun2socksPID)
	fmt.Fprintf(out, "logs: %s\n", paths.LogDir)
	return nil
}

func Disconnect(paths config.Paths, dryRun bool, out io.Writer) error {
	if _, err := os.Stat(paths.RuntimeFile); os.IsNotExist(err) {
		fmt.Fprintln(out, "not connected")
		return nil
	}
	state, err := LoadState(paths.RuntimeFile)
	if err != nil {
		return err
	}
	if dryRun {
		printDisconnectPlan(out, state)
		return nil
	}
	if state.DNS != nil && state.DNS.Enabled {
		_ = dns.Revert(state.DNS.Link)
	}
	if state.Tun2socksPID > 0 {
		_ = privileged("kill", strconv.Itoa(state.Tun2socksPID))
	}
	if state.XrayPID > 0 {
		_ = exec.Command("kill", strconv.Itoa(state.XrayPID)).Run()
	}
	_ = privileged("ip", "route", "del", "0.0.0.0/1", "dev", state.TunName)
	_ = privileged("ip", "route", "del", "128.0.0.0/1", "dev", state.TunName)
	if state.ServerRoute != nil {
		_ = privileged("ip", "route", "del", state.ServerRoute.IP+"/32")
	}
	_ = privileged("ip", "link", "delete", state.TunName)
	if err := os.Remove(paths.RuntimeFile); err != nil {
		return fmt.Errorf("remove runtime state %s: %w", paths.RuntimeFile, err)
	}
	fmt.Fprintf(out, "disconnected: %s\n", state.ProfileName)
	return nil
}

func StatusText(paths config.Paths) (string, error) {
	if _, err := os.Stat(paths.RuntimeFile); os.IsNotExist(err) {
		return "not connected", nil
	}
	state, err := LoadState(paths.RuntimeFile)
	if err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("profile: %s (%s)", state.ProfileName, state.ProfileID),
		fmt.Sprintf("xray: pid %d %s", state.XrayPID, runningText(state.XrayPID)),
	}
	if state.Tun2socksPID > 0 {
		lines = append(lines, fmt.Sprintf("tun2socks: pid %d %s", state.Tun2socksPID, runningText(state.Tun2socksPID)))
	}
	lines = append(lines, fmt.Sprintf("tun: %s %s", state.TunName, state.TunCIDR))
	lines = append(lines, fmt.Sprintf("socks: 127.0.0.1:%d", state.SocksPort))
	if state.ServerRoute != nil {
		gateway := state.ServerRoute.Gateway
		if gateway == "" {
			gateway = "<direct>"
		}
		lines = append(lines, fmt.Sprintf("server route: %s/32 via %s dev %s", state.ServerRoute.IP, gateway, state.ServerRoute.Dev))
	}
	if state.DNS != nil && state.DNS.Enabled {
		lines = append(lines, fmt.Sprintf("dns: %s via %s -> %s", state.DNS.Link, state.DNS.Manager, strings.Join(state.DNS.Servers, ", ")))
	}
	return strings.Join(lines, "\n"), nil
}

func TailLogs(logDir string, lines int) (string, error) {
	var out strings.Builder
	for _, name := range []string{"xray.log", "tun2socks.log"} {
		path := filepath.Join(logDir, name)
		out.WriteString("== " + name + " ==\n")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			out.WriteString("log file does not exist\n")
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read log %s: %w", path, err)
		}
		all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		start := 0
		if len(all) > lines {
			start = len(all) - lines
		}
		for _, line := range all[start:] {
			if line != "" {
				out.WriteString(line + "\n")
			}
		}
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read runtime state %s: %w", path, err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse runtime state %s: %w", path, err)
	}
	return state, nil
}

func SaveState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func activeRuntime(path string) (bool, error) {
	state, err := LoadState(path)
	if err != nil {
		return false, err
	}
	return processAlive(state.XrayPID) || (state.Tun2socksPID > 0 && processAlive(state.Tun2socksPID)), nil
}

func setupTUNAndRoutes(opts ConnectOptions, route *ServerRoute) error {
	if linkExists(opts.TunName) {
		if err := privileged("ip", "link", "delete", opts.TunName); err != nil {
			return err
		}
	}
	if err := privileged("ip", "tuntap", "add", "dev", opts.TunName, "mode", "tun"); err != nil {
		return err
	}
	if err := privileged("ip", "addr", "replace", opts.TunCIDR, "dev", opts.TunName); err != nil {
		return err
	}
	if err := privileged("ip", "link", "set", "dev", opts.TunName, "up"); err != nil {
		return err
	}
	if route != nil {
		cidr := route.IP + "/32"
		if route.Gateway != "" {
			if err := privileged("ip", "route", "replace", cidr, "via", route.Gateway, "dev", route.Dev); err != nil {
				return err
			}
		} else if err := privileged("ip", "route", "replace", cidr, "dev", route.Dev); err != nil {
			return err
		}
	}
	if err := privileged("ip", "route", "replace", "0.0.0.0/1", "dev", opts.TunName); err != nil {
		return err
	}
	return privileged("ip", "route", "replace", "128.0.0.0/1", "dev", opts.TunName)
}

func cleanupPartialConnect(opts ConnectOptions, xrayPID, tunPID int, route *ServerRoute, dnsConfigured bool) {
	if dnsConfigured {
		_ = dns.Revert(opts.TunName)
	}
	if tunPID > 0 {
		_ = privileged("kill", strconv.Itoa(tunPID))
	}
	if xrayPID > 0 {
		_ = exec.Command("kill", strconv.Itoa(xrayPID)).Run()
	}
	_ = privileged("ip", "route", "del", "0.0.0.0/1", "dev", opts.TunName)
	_ = privileged("ip", "route", "del", "128.0.0.0/1", "dev", opts.TunName)
	if route != nil {
		_ = privileged("ip", "route", "del", route.IP+"/32")
	}
	_ = privileged("ip", "link", "delete", opts.TunName)
}

func printConnectPlan(out io.Writer, paths config.Paths, p profile.Profile, opts ConnectOptions, route *ServerRoute) {
	fmt.Fprintf(out, "dry-run connect: %s\n", p.Name)
	fmt.Fprintf(out, "xray: %s -c %s\n", opts.XrayBin, paths.GeneratedConfig)
	fmt.Fprintf(out, "sudo ip tuntap add dev %s mode tun\n", opts.TunName)
	fmt.Fprintf(out, "sudo ip addr replace %s dev %s\n", opts.TunCIDR, opts.TunName)
	fmt.Fprintf(out, "sudo ip link set dev %s up\n", opts.TunName)
	if route != nil {
		if route.Gateway != "" {
			fmt.Fprintf(out, "sudo ip route replace %s/32 via %s dev %s\n", route.IP, route.Gateway, route.Dev)
		} else {
			fmt.Fprintf(out, "sudo ip route replace %s/32 dev %s\n", route.IP, route.Dev)
		}
	}
	fmt.Fprintf(out, "sudo ip route replace 0.0.0.0/1 dev %s\n", opts.TunName)
	fmt.Fprintf(out, "sudo ip route replace 128.0.0.0/1 dev %s\n", opts.TunName)
	fmt.Fprintf(out, "sudo %s -device tun://%s -proxy socks5://127.0.0.1:%d\n", opts.Tun2socksBin, opts.TunName, opts.SocksPort)
	if opts.DNSEnabled {
		for _, command := range dns.SetupCommands(opts.TunName, opts.DNSServers) {
			fmt.Fprintln(out, command)
		}
	}
}

func printDisconnectPlan(out io.Writer, state State) {
	fmt.Fprintf(out, "dry-run disconnect: %s\n", state.ProfileName)
	if state.DNS != nil && state.DNS.Enabled {
		fmt.Fprintln(out, dns.RevertCommand(state.DNS.Link))
	}
	if state.Tun2socksPID > 0 {
		fmt.Fprintf(out, "sudo kill %d\n", state.Tun2socksPID)
	}
	fmt.Fprintf(out, "kill %d\n", state.XrayPID)
	fmt.Fprintf(out, "sudo ip route del 0.0.0.0/1 dev %s\n", state.TunName)
	fmt.Fprintf(out, "sudo ip route del 128.0.0.0/1 dev %s\n", state.TunName)
	if state.ServerRoute != nil {
		fmt.Fprintf(out, "sudo ip route del %s/32\n", state.ServerRoute.IP)
	}
	fmt.Fprintf(out, "sudo ip link delete %s\n", state.TunName)
}

func resolveServerRoute(address string) *ServerRoute {
	route, err := DefaultRouteInfo()
	if err != nil {
		return nil
	}
	ip, err := ResolveIPv4(address)
	if err != nil {
		return nil
	}
	return &ServerRoute{IP: ip, Gateway: route.Gateway, Dev: route.Dev}
}

func DefaultRouteInfo() (DefaultRoute, error) {
	output, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return DefaultRoute{}, fmt.Errorf("inspect default route: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "default ") {
			continue
		}
		parts := strings.Fields(line)
		route := DefaultRoute{}
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "via" {
				route.Gateway = parts[i+1]
			}
			if parts[i] == "dev" {
				route.Dev = parts[i+1]
			}
		}
		if route.Dev == "" {
			return DefaultRoute{}, errors.New("default route has no dev")
		}
		return route, nil
	}
	return DefaultRoute{}, errors.New("no default route found")
}

func ResolveIPv4(address string) (string, error) {
	if ip := net.ParseIP(address); ip != nil {
		if ip.To4() == nil {
			return "", fmt.Errorf("not an IPv4 address: %s", address)
		}
		return ip.String(), nil
	}
	output, err := exec.Command("getent", "ahostsv4", address).Output()
	if err != nil {
		return "", fmt.Errorf("resolve server address %s: %w", address, err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if ip := net.ParseIP(fields[0]); ip != nil && ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no IPv4 address found for %s", address)
}

func logFiles(logDir, name string) (*os.File, *os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.Create(filepath.Join(logDir, name))
	if err != nil {
		return nil, nil, fmt.Errorf("create log %s: %w", name, err)
	}
	clone, err := file.Seek(0, io.SeekCurrent)
	_ = clone
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, file, nil
}

func linkExists(name string) bool {
	return exec.Command("ip", "link", "show", name).Run() == nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func runningText(pid int) string {
	if processAlive(pid) {
		return "running"
	}
	return "stopped"
}

func privileged(args ...string) error {
	cmd := privilegedCommand(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("privileged command failed %q: %w", strings.Join(args, " "), err)
	}
	return nil
}

func privilegedCommand(program string, args ...string) *exec.Cmd {
	if isRoot() {
		return exec.Command(program, args...)
	}
	all := append([]string{program}, args...)
	return exec.Command("sudo", all...)
}

func isRoot() bool {
	return os.Geteuid() == 0
}
