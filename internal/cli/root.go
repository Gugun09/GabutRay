package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gabutray/internal/config"
	"gabutray/internal/daemon"
	"gabutray/internal/doctor"
	"gabutray/internal/latency"
	"gabutray/internal/menu"
	"gabutray/internal/profile"
	"gabutray/internal/runtime"
	"gabutray/internal/service"
	"gabutray/internal/xray"
)

type app struct {
	configFile string
	socket     string
}

func NewRootCommand() *cobra.Command {
	a := &app{socket: config.DefaultSocket}
	cmd := &cobra.Command{
		Use:   "gabutray",
		Short: "Linux Xray/V2Ray VPN-style CLI with TUN mode",
	}
	cmd.PersistentFlags().StringVar(&a.configFile, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&a.socket, "socket", config.DefaultSocket, "daemon Unix socket path")

	cmd.AddCommand(
		a.profileCommand(),
		a.connectCommand(),
		a.disconnectCommand(),
		a.statusCommand(),
		a.logsCommand(),
		a.menuCommand(),
		a.daemonCommand(),
		a.serviceCommand(),
		a.doctorCommand(),
		a.configCommand(),
	)
	return cmd
}

func (a *app) load() (config.Paths, config.Config, error) {
	paths, err := config.UserPaths(a.configFile)
	if err != nil {
		return config.Paths{}, config.Config{}, err
	}
	cfg, err := config.Load(paths, a.configFile)
	if err != nil {
		return config.Paths{}, config.Config{}, err
	}
	return paths, cfg, nil
}

func (a *app) profileCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage VPN profiles"}
	var name string
	add := &cobra.Command{
		Use:   "add <share-link>",
		Short: "Import a VLESS, VMess, or Trojan share link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			item, err := profile.ParseShareLink(args[0])
			if err != nil {
				return err
			}
			if name != "" {
				item.Name = name
			}
			imported, err := profile.Add(paths.ProfilesFile, item)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported: %s (%s) -> %s:%d\n", imported.ID, imported.Protocol, imported.Address, imported.Port)
			return nil
		},
	}
	add.Flags().StringVar(&name, "name", "", "profile display name")

	list := &cobra.Command{
		Use:   "list",
		Short: "List imported profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			items, err := profile.LoadAll(paths.ProfilesFile)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no profiles imported")
				return nil
			}
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s:%d\n", item.ID, item.Name, item.Protocol, item.Address, item.Port)
			}
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <id-or-name>",
		Short: "Remove an imported profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			removed, err := profile.Remove(paths.ProfilesFile, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed: %s (%s)\n", removed.ID, removed.Name)
			return nil
		},
	}

	inspect := &cobra.Command{
		Use:   "inspect <id-or-name-or-share-link>",
		Short: "Print the generated Xray outbound config for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			item, err := profileForInspect(paths.ProfilesFile, args[0])
			if err != nil {
				return err
			}
			outbound, err := xray.BuildOutbound(item)
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(outbound, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	var timeoutValue string
	testCmd := &cobra.Command{
		Use:   "test [id-or-name]",
		Short: "Test TCP latency for imported profiles",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			timeout, err := parsePositiveDuration(timeoutValue)
			if err != nil {
				return err
			}

			var items []profile.Profile
			if len(args) == 1 {
				item, err := profile.Find(paths.ProfilesFile, args[0])
				if err != nil {
					return err
				}
				items = []profile.Profile{item}
			} else {
				items, err = profile.LoadAll(paths.ProfilesFile)
				if err != nil {
					return err
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), latency.FormatResults(latency.CheckAll(items, timeout)))
			return nil
		},
	}
	testCmd.Flags().StringVar(&timeoutValue, "timeout", "3s", "TCP connect timeout")

	cmd.AddCommand(add, list, remove, inspect, testCmd)
	return cmd
}

func (a *app) connectCommand() *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "connect <id-or-name-or-share-link>",
		Short: "Connect a profile through the root daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, cfg, err := a.load()
			if err != nil {
				return err
			}
			item, err := profile.FromTarget(paths.ProfilesFile, args[0])
			if err != nil {
				return err
			}
			options := runtime.OptionsFromConfig(cfg, dryRun)
			if dryRun {
				if force {
					_ = runtime.Disconnect(paths, true, cmd.OutOrStdout())
				}
				return runtime.Connect(paths, item, options, cmd.OutOrStdout())
			}
			response, err := daemon.RequestDaemon(a.socket, daemon.Request{
				Action:  "connect",
				Profile: &item,
				Options: options,
				Force:   force,
			})
			if err != nil {
				return err
			}
			printResponse(cmd, response)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "disconnect an existing runtime before connecting")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands and generated config without changing routes")
	return cmd
}

func (a *app) disconnectCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect the active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := a.load()
			if err != nil {
				return err
			}
			if dryRun {
				return runtime.Disconnect(paths, true, cmd.OutOrStdout())
			}
			response, err := daemon.RequestDaemon(a.socket, daemon.Request{Action: "disconnect"})
			if err != nil {
				return err
			}
			printResponse(cmd, response)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print cleanup commands without changing routes")
	return cmd
}

func (a *app) statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show runtime status",
		RunE: func(cmd *cobra.Command, args []string) error {
			response, err := daemon.RequestDaemon(a.socket, daemon.Request{Action: "status"})
			if err != nil {
				paths, _, loadErr := a.load()
				if loadErr != nil {
					return err
				}
				text, statusErr := runtime.StatusText(paths)
				if statusErr != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), text)
				return nil
			}
			printResponse(cmd, response)
			return nil
		},
	}
}

func (a *app) logsCommand() *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			response, err := daemon.RequestDaemon(a.socket, daemon.Request{Action: "logs", Lines: lines})
			if err != nil {
				return err
			}
			printResponse(cmd, response)
			return nil
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 80, "number of log lines per process")
	return cmd
}

func (a *app) menuCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "menu",
		Short: "Open an interactive beginner-friendly menu",
		RunE: func(cmd *cobra.Command, args []string) error {
			return menu.Run(menu.Options{
				ConfigFile: a.configFile,
				Socket:     a.socket,
			})
		},
	}
}

func (a *app) daemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the background root daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Run(a.socket, config.DaemonPaths(), cmd.OutOrStdout())
		},
	}
}

func (a *app) serviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Print, install, or uninstall the systemd service"}
	printCmd := &cobra.Command{
		Use:   "print",
		Short: "Print the systemd unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			unit, err := service.CurrentUnit(a.socket)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), unit)
			return nil
		},
	}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install and start the systemd service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Install(a.socket)
		},
	}
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Disable and remove the systemd service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Uninstall()
		},
	}
	cmd.AddCommand(printCmd, installCmd, uninstallCmd)
	return cmd
}

func (a *app) doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local dependencies and daemon state",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, cfg, err := a.load()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), doctor.Report(a.socket, paths, cfg))
			return nil
		},
	}
}

func (a *app) configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show or update Gabutray config"}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show saved config",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := a.load()
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	var next config.Config
	var dnsServers string
	set := &cobra.Command{
		Use:   "set",
		Short: "Update binary and network defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, cfg, err := a.load()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("xray-bin") {
				cfg.XrayBin = next.XrayBin
			}
			if cmd.Flags().Changed("tun2socks-bin") {
				cfg.Tun2socksBin = next.Tun2socksBin
			}
			if cmd.Flags().Changed("socks-port") {
				cfg.SocksPort = next.SocksPort
			}
			if cmd.Flags().Changed("tun-name") {
				cfg.TunName = next.TunName
			}
			if cmd.Flags().Changed("tun-cidr") {
				cfg.TunCIDR = next.TunCIDR
			}
			if cmd.Flags().Changed("dns-enabled") {
				cfg.DNSEnabled = next.DNSEnabled
			}
			if cmd.Flags().Changed("dns-strict") {
				cfg.DNSStrict = next.DNSStrict
			}
			if cmd.Flags().Changed("dns-servers") {
				servers, err := config.ParseDNSServers(dnsServers)
				if err != nil {
					return err
				}
				cfg.DNSServers = servers
			}
			if err := config.Save(paths, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved config: %s\n", paths.ConfigFile)
			return nil
		},
	}
	set.Flags().StringVar(&next.XrayBin, "xray-bin", "", "xray binary path")
	set.Flags().StringVar(&next.Tun2socksBin, "tun2socks-bin", "", "tun2socks binary path")
	set.Flags().IntVar(&next.SocksPort, "socks-port", 0, "local SOCKS port")
	set.Flags().StringVar(&next.TunName, "tun-name", "", "TUN interface name")
	set.Flags().StringVar(&next.TunCIDR, "tun-cidr", "", "TUN CIDR")
	set.Flags().BoolVar(&next.DNSEnabled, "dns-enabled", false, "enable automatic DNS anti-leak")
	set.Flags().BoolVar(&next.DNSStrict, "dns-strict", false, "fail connect if DNS anti-leak cannot be configured")
	set.Flags().StringVar(&dnsServers, "dns-servers", "", "comma-separated DNS server IPs")

	cmd.AddCommand(show, set)
	return cmd
}

func printResponse(cmd *cobra.Command, response daemon.Response) {
	if response.Message != "" {
		fmt.Fprintln(cmd.OutOrStdout(), response.Message)
	}
	if response.Data != "" {
		fmt.Fprintln(cmd.OutOrStdout(), response.Data)
	}
}

func profileForInspect(path, target string) (profile.Profile, error) {
	if strings.HasPrefix(target, "vless://") || strings.HasPrefix(target, "vmess://") || strings.HasPrefix(target, "trojan://") {
		return profile.ParseShareLink(target)
	}
	return profile.Find(path, target)
}

func parsePositiveDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", value, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("timeout must be positive: %s", value)
	}
	return duration, nil
}
