package dns

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const ManagerSystemdResolved = "systemd-resolved"

type State struct {
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Manager string   `json:"manager" yaml:"manager"`
	Link    string   `json:"link" yaml:"link"`
	Servers []string `json:"servers" yaml:"servers"`
}

func Available() bool {
	_, err := exec.LookPath("resolvectl")
	return err == nil
}

func Setup(link string, servers []string) (State, error) {
	if !Available() {
		return State{}, fmt.Errorf("resolvectl not found")
	}
	if len(servers) == 0 {
		return State{}, fmt.Errorf("no DNS servers configured")
	}
	if err := privileged("resolvectl", append([]string{"dns", link}, servers...)...); err != nil {
		return State{}, err
	}
	if err := privileged("resolvectl", "domain", link, "~."); err != nil {
		_ = Revert(link)
		return State{}, err
	}
	if err := privileged("resolvectl", "default-route", link, "yes"); err != nil {
		_ = Revert(link)
		return State{}, err
	}
	return State{
		Enabled: true,
		Manager: ManagerSystemdResolved,
		Link:    link,
		Servers: append([]string(nil), servers...),
	}, nil
}

func Revert(link string) error {
	if !Available() {
		return fmt.Errorf("resolvectl not found")
	}
	return privileged("resolvectl", "revert", link)
}

func SetupCommands(link string, servers []string) []string {
	joined := strings.Join(servers, " ")
	return []string{
		fmt.Sprintf("sudo resolvectl dns %s %s", link, joined),
		fmt.Sprintf("sudo resolvectl domain %s '~.'", link),
		fmt.Sprintf("sudo resolvectl default-route %s yes", link),
	}
}

func RevertCommand(link string) string {
	return fmt.Sprintf("sudo resolvectl revert %s", link)
}

func StatusText(enabled bool, servers []string) string {
	if !enabled {
		return "dns: disabled"
	}
	if !Available() {
		return "dns: enabled, missing resolvectl"
	}
	return "dns: enabled via systemd-resolved -> " + strings.Join(servers, ", ")
}

func privileged(program string, args ...string) error {
	cmd := privilegedCommand(program, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("privileged DNS command failed %q: %w", strings.Join(append([]string{program}, args...), " "), err)
	}
	return nil
}

func privilegedCommand(program string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(program, args...)
	}
	all := append([]string{program}, args...)
	return exec.Command("sudo", all...)
}
