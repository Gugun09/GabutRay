package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const UnitPath = "/etc/systemd/system/gabutrayd.service"

func Unit(executable, socket string) string {
	return fmt.Sprintf(`[Unit]
Description=Gabutray Xray/V2Ray TUN daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --socket %s daemon
Restart=on-failure
RestartSec=2
RuntimeDirectory=gabutray

[Install]
WantedBy=multi-user.target
`, executable, socket)
}

func CurrentUnit(socket string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return Unit(exe, socket), nil
}

func Install(socket string) error {
	unit, err := CurrentUnit(socket)
	if err != nil {
		return err
	}
	cmd := exec.Command("sudo", "tee", UnitPath)
	cmd.Stdin = strings.NewReader(unit)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}
	if err := sudo("systemctl", "daemon-reload"); err != nil {
		return err
	}
	return sudo("systemctl", "enable", "--now", "gabutrayd.service")
}

func Uninstall() error {
	_ = sudo("systemctl", "disable", "--now", "gabutrayd.service")
	if err := sudo("rm", "-f", UnitPath); err != nil {
		return err
	}
	return sudo("systemctl", "daemon-reload")
}

func sudo(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}
