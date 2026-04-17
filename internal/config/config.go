package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	DefaultSocket = "/run/gabutrayd.sock"
)

type Config struct {
	XrayBin      string   `mapstructure:"xray_bin" yaml:"xray_bin" json:"xray_bin"`
	Tun2socksBin string   `mapstructure:"tun2socks_bin" yaml:"tun2socks_bin" json:"tun2socks_bin"`
	SocksPort    int      `mapstructure:"socks_port" yaml:"socks_port" json:"socks_port"`
	TunName      string   `mapstructure:"tun_name" yaml:"tun_name" json:"tun_name"`
	TunCIDR      string   `mapstructure:"tun_cidr" yaml:"tun_cidr" json:"tun_cidr"`
	DNSEnabled   bool     `mapstructure:"dns_enabled" yaml:"dns_enabled" json:"dns_enabled"`
	DNSServers   []string `mapstructure:"dns_servers" yaml:"dns_servers" json:"dns_servers"`
	DNSStrict    bool     `mapstructure:"dns_strict" yaml:"dns_strict" json:"dns_strict"`
}

type Paths struct {
	Root            string
	ConfigFile      string
	ProfilesFile    string
	GeneratedConfig string
	RuntimeFile     string
	LogDir          string
}

func DefaultConfig() Config {
	return Config{
		XrayBin:      defaultEnginePath("xray"),
		Tun2socksBin: defaultEnginePath("tun2socks"),
		SocksPort:    10808,
		TunName:      "gabutray0",
		TunCIDR:      "198.18.0.1/15",
		DNSEnabled:   true,
		DNSServers:   []string{"1.1.1.1", "1.0.0.1"},
		DNSStrict:    true,
	}
}

func UserPaths(configFile string) (Paths, error) {
	root, err := userRoot(configFile)
	if err != nil {
		return Paths{}, err
	}
	return buildPaths(root, configFile), nil
}

func DaemonPaths() Paths {
	return Paths{
		Root:            "/run/gabutray",
		ConfigFile:      "/run/gabutray/config.yaml",
		ProfilesFile:    "/run/gabutray/profiles.yaml",
		GeneratedConfig: "/run/gabutray/xray.generated.json",
		RuntimeFile:     "/run/gabutray/runtime.json",
		LogDir:          "/run/gabutray/logs",
	}
}

func Ensure(paths Paths) error {
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return fmt.Errorf("create config directory %s: %w", paths.Root, err)
	}
	if err := os.MkdirAll(paths.LogDir, 0o755); err != nil {
		return fmt.Errorf("create log directory %s: %w", paths.LogDir, err)
	}
	return nil
}

func Load(paths Paths, configFile string) (Config, error) {
	if err := Ensure(paths); err != nil {
		return Config{}, err
	}

	cfg := DefaultConfig()
	v := viper.New()
	setDefaults(v, cfg)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("GABUTRAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(paths.Root)
	}

	if err := v.ReadInConfig(); err != nil {
		var lookup viper.ConfigFileNotFoundError
		if !errors.As(err, &lookup) && !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func Save(paths Paths, cfg Config) error {
	if err := Ensure(paths); err != nil {
		return err
	}
	v := viper.New()
	v.SetConfigType("yaml")
	v.Set("xray_bin", cfg.XrayBin)
	v.Set("tun2socks_bin", cfg.Tun2socksBin)
	v.Set("socks_port", cfg.SocksPort)
	v.Set("tun_name", cfg.TunName)
	v.Set("tun_cidr", cfg.TunCIDR)
	v.Set("dns_enabled", cfg.DNSEnabled)
	v.Set("dns_servers", cfg.DNSServers)
	v.Set("dns_strict", cfg.DNSStrict)
	if err := v.WriteConfigAs(paths.ConfigFile); err != nil {
		return fmt.Errorf("write config %s: %w", paths.ConfigFile, err)
	}
	return nil
}

func setDefaults(v *viper.Viper, cfg Config) {
	v.SetDefault("xray_bin", cfg.XrayBin)
	v.SetDefault("tun2socks_bin", cfg.Tun2socksBin)
	v.SetDefault("socks_port", cfg.SocksPort)
	v.SetDefault("tun_name", cfg.TunName)
	v.SetDefault("tun_cidr", cfg.TunCIDR)
	v.SetDefault("dns_enabled", cfg.DNSEnabled)
	v.SetDefault("dns_servers", cfg.DNSServers)
	v.SetDefault("dns_strict", cfg.DNSStrict)
}

func ParseDNSServers(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	servers := make([]string, 0, len(parts))
	for _, part := range parts {
		server := strings.TrimSpace(part)
		if server == "" {
			continue
		}
		ip := net.ParseIP(server)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("invalid DNS server IPv4 address: %s", server)
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return nil, errors.New("at least one DNS server is required")
	}
	return servers, nil
}

func userRoot(configFile string) (string, error) {
	if configFile != "" {
		return filepath.Dir(configFile), nil
	}
	if root := os.Getenv("GABUTRAY_HOME"); root != "" {
		return root, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "gabutray"), nil
}

func buildPaths(root, configFile string) Paths {
	if configFile == "" {
		configFile = filepath.Join(root, "config.yaml")
	}
	return Paths{
		Root:            root,
		ConfigFile:      configFile,
		ProfilesFile:    filepath.Join(root, "profiles.yaml"),
		GeneratedConfig: filepath.Join(root, "xray.generated.json"),
		RuntimeFile:     filepath.Join(root, "runtime.json"),
		LogDir:          filepath.Join(root, "logs"),
	}
}

func defaultEnginePath(name string) string {
	candidate := filepath.Join("/opt/gabutray/engines", name)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return name
}
