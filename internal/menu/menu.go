package menu

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gabutray/internal/config"
	"gabutray/internal/daemon"
	"gabutray/internal/doctor"
	"gabutray/internal/latency"
	"gabutray/internal/profile"
	"gabutray/internal/runtime"
)

type Options struct {
	ConfigFile string
	Socket     string
}

type action int

const (
	actionSetup action = iota
	actionAddProfile
	actionConnect
	actionTestProfile
	actionDisconnect
	actionStatus
	actionLogs
	actionDoctor
	actionQuit
)

type stage int

const (
	stageHome stage = iota
	stageInputLink
	stageInputName
	stageSelectProfile
	stageMessage
)

type menuItem struct {
	title string
	desc  string
	act   action
}

type model struct {
	opts       Options
	paths      config.Paths
	cfg        config.Config
	stage      stage
	cursor     int
	items      []menuItem
	profiles   []profile.Profile
	input      textinput.Model
	pending    profile.Profile
	message    string
	errMessage string
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	choiceStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	panelStyle   = lipgloss.NewStyle().Padding(1, 2)
)

func Run(opts Options) error {
	paths, err := config.UserPaths(opts.ConfigFile)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths, opts.ConfigFile)
	if err != nil {
		return err
	}
	m := newModel(opts, paths, cfg)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(opts Options, paths config.Paths, cfg config.Config) model {
	input := textinput.New()
	input.CharLimit = 4096
	input.Width = 90
	input.Prompt = "> "
	return model{
		opts:  opts,
		paths: paths,
		cfg:   cfg,
		stage: stageHome,
		items: []menuItem{
			{"Quick Setup", "Cek kesiapan service dan dependency", actionSetup},
			{"Tambah Profile", "Paste link vless://, vmess://, atau trojan://", actionAddProfile},
			{"Pilih & Connect", "Pilih akun lalu hubungkan VPN", actionConnect},
			{"Test Profile", "Cek latency TCP tiap akun tanpa connect", actionTestProfile},
			{"Disconnect", "Putuskan koneksi VPN aktif", actionDisconnect},
			{"Status", "Lihat apakah VPN sedang terhubung", actionStatus},
			{"Logs", "Lihat log Xray dan tun2socks terakhir", actionLogs},
			{"Doctor", "Pemeriksaan teknis lengkap", actionDoctor},
			{"Keluar", "Tutup menu", actionQuit},
		},
		input: input,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m.back()
		}

		switch m.stage {
		case stageHome:
			return m.updateHome(msg)
		case stageInputLink, stageInputName:
			return m.updateInput(msg)
		case stageSelectProfile:
			return m.updateProfileList(msg)
		case stageMessage:
			return m.updateMessage(msg)
		}
	}
	return m, nil
}

func (m model) View() string {
	var body string
	switch m.stage {
	case stageHome:
		body = m.viewHome()
	case stageInputLink:
		body = m.viewInput("Tambah Profile", "Paste share link dari provider VPN/proxy.")
	case stageInputName:
		body = m.viewInput("Nama Profile", "Kosongkan untuk memakai nama dari link.")
	case stageSelectProfile:
		body = m.viewProfileList()
	case stageMessage:
		body = m.viewMessage()
	}
	return panelStyle.Render(body)
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		item := m.items[m.cursor]
		return m.runAction(item.act)
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		if m.stage == stageInputLink {
			if value == "" {
				m.errMessage = "Link tidak boleh kosong."
				return m, nil
			}
			parsed, err := profile.ParseShareLink(value)
			if err != nil {
				m.errMessage = friendlyError("Link tidak bisa dibaca.", err)
				return m, nil
			}
			m.pending = parsed
			m.stage = stageInputName
			m.errMessage = ""
			m.input.Reset()
			m.input.Placeholder = parsed.Name
			m.input.Focus()
			return m, textinput.Blink
		}
		if value != "" {
			m.pending.Name = value
		}
		imported, err := profile.Add(m.paths.ProfilesFile, m.pending)
		if err != nil {
			return m.withMessage(friendlyError("Profile gagal disimpan.", err)), nil
		}
		return m.withMessage(fmt.Sprintf("Profile tersimpan.\n\nNama: %s\nProtocol: %s\nServer: %s:%d", imported.Name, imported.Protocol, imported.Address, imported.Port)), nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateProfileList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.back()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.profiles) == 0 {
			return m.withMessage("Belum ada profile. Pilih Tambah Profile dulu."), nil
		}
		item := m.profiles[m.cursor]
		response, err := daemon.RequestDaemon(m.opts.Socket, daemon.Request{
			Action:  "connect",
			Profile: &item,
			Options: runtime.OptionsFromConfig(m.cfg, false),
			Force:   true,
		})
		if err != nil {
			return m.withMessage(friendlyError("Gagal connect. Service latar belakang mungkin belum aktif.\n\nJalankan:\n  gabutray service install\n\nLalu coba lagi.", err)), nil
		}
		return m.withMessage(responseText(response)), nil
	}
	return m, nil
}

func (m model) updateMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter", "b", "backspace":
		return m.back()
	}
	return m, nil
}

func (m model) runAction(act action) (tea.Model, tea.Cmd) {
	switch act {
	case actionSetup:
		report := doctor.Report(m.opts.Socket, m.paths, m.cfg)
		msg := "Quick Setup\n\n" + beginnerDoctorSummary(report) + "\n\nDetail:\n" + report + "\n\nUntuk memasang service latar belakang:\n  gabutray service install"
		return m.withMessage(msg), nil
	case actionAddProfile:
		m.stage = stageInputLink
		m.errMessage = ""
		m.input.Reset()
		m.input.Placeholder = "vless://... atau vmess://... atau trojan://..."
		m.input.Focus()
		return m, textinput.Blink
	case actionConnect:
		items, err := profile.LoadAll(m.paths.ProfilesFile)
		if err != nil {
			return m.withMessage(friendlyError("Tidak bisa membaca daftar profile.", err)), nil
		}
		if len(items) == 0 {
			return m.withMessage("Belum ada profile.\n\nPilih Tambah Profile dulu, lalu paste link akun dari provider."), nil
		}
		m.profiles = items
		m.cursor = 0
		m.stage = stageSelectProfile
		return m, nil
	case actionTestProfile:
		items, err := profile.LoadAll(m.paths.ProfilesFile)
		if err != nil {
			return m.withMessage(friendlyError("Tidak bisa membaca daftar profile.", err)), nil
		}
		if len(items) == 0 {
			return m.withMessage("Belum ada profile.\n\nPilih Tambah Profile dulu, lalu paste link akun dari provider."), nil
		}
		results := latency.CheckAll(items, 3*time.Second)
		return m.withMessage("Test Profile\n\n" + latency.FormatResults(results)), nil
	case actionDisconnect:
		response, err := daemon.RequestDaemon(m.opts.Socket, daemon.Request{Action: "disconnect"})
		if err != nil {
			return m.withMessage(friendlyError("Gagal disconnect. Service latar belakang mungkin belum aktif.", err)), nil
		}
		return m.withMessage(responseText(response)), nil
	case actionStatus:
		response, err := daemon.RequestDaemon(m.opts.Socket, daemon.Request{Action: "status"})
		if err != nil {
			text, statusErr := runtime.StatusText(m.paths)
			if statusErr == nil {
				return m.withMessage("Status\n\n" + friendlyStatus(text)), nil
			}
			return m.withMessage(friendlyError("Status tidak bisa dibaca.", err)), nil
		}
		return m.withMessage("Status\n\n" + friendlyStatus(response.Message)), nil
	case actionLogs:
		response, err := daemon.RequestDaemon(m.opts.Socket, daemon.Request{Action: "logs", Lines: 80})
		if err != nil {
			return m.withMessage(friendlyError("Log tidak bisa dibaca. Service latar belakang mungkin belum aktif.", err)), nil
		}
		return m.withMessage("Logs\n\n" + response.Message), nil
	case actionDoctor:
		return m.withMessage(doctor.Report(m.opts.Socket, m.paths, m.cfg)), nil
	case actionQuit:
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m model) back() (tea.Model, tea.Cmd) {
	m.stage = stageHome
	m.cursor = 0
	m.errMessage = ""
	m.message = ""
	m.input.Blur()
	return m, nil
}

func (m model) withMessage(message string) model {
	m.stage = stageMessage
	m.cursor = 0
	m.message = message
	m.errMessage = ""
	m.input.Blur()
	return m
}

func (m model) viewHome() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Gabutray Menu") + "\n")
	b.WriteString(mutedStyle.Render("Pilih dengan panah lalu Enter. Tekan q untuk keluar.") + "\n\n")
	for i, item := range m.items {
		cursor := "  "
		style := choiceStyle
		if i == m.cursor {
			cursor = "> "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor+item.title) + "\n")
		b.WriteString(mutedStyle.Render("    "+item.desc) + "\n")
	}
	return b.String()
}

func (m model) viewInput(title, help string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n")
	b.WriteString(mutedStyle.Render(help) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	if m.errMessage != "" {
		b.WriteString("\n" + errorStyle.Render(m.errMessage) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("Enter untuk lanjut. Esc untuk kembali.") + "\n")
	return b.String()
}

func (m model) viewProfileList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Pilih Profile") + "\n")
	b.WriteString(mutedStyle.Render("Enter untuk connect. Esc untuk kembali.") + "\n\n")
	for i, item := range m.profiles {
		cursor := "  "
		style := choiceStyle
		if i == m.cursor {
			cursor = "> "
			style = cursorStyle
		}
		line := fmt.Sprintf("%s%s  %s:%d", cursor, item.Name, item.Address, item.Port)
		b.WriteString(style.Render(line) + "\n")
		b.WriteString(mutedStyle.Render("    "+string(item.Protocol)+" | id: "+item.ID) + "\n")
	}
	return b.String()
}

func (m model) viewMessage() string {
	body := m.message
	if strings.Contains(strings.ToLower(body), "gagal") || strings.Contains(strings.ToLower(body), "error") {
		body = errorStyle.Render(body)
	} else {
		body = successStyle.Render(body)
	}
	return body + "\n\n" + mutedStyle.Render("Enter/Esc untuk kembali. q untuk keluar.")
}

func beginnerDoctorSummary(report string) string {
	var missing []string
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "missing:") {
			missing = append(missing, strings.TrimSpace(strings.TrimPrefix(line, "missing:")))
		}
	}
	if len(missing) == 0 && !strings.Contains(report, "warn: daemon socket not found") {
		return "Semua komponen utama terlihat siap."
	}
	var b strings.Builder
	if len(missing) > 0 {
		b.WriteString("Ada komponen yang belum ditemukan: " + strings.Join(missing, ", ") + ".")
	} else {
		b.WriteString("Komponen utama ditemukan.")
	}
	if strings.Contains(report, "warn: daemon socket not found") {
		b.WriteString("\nService latar belakang belum aktif atau belum terpasang.")
	}
	return b.String()
}

func friendlyStatus(text string) string {
	if strings.TrimSpace(text) == "not connected" {
		return "Belum terhubung."
	}
	return text
}

func responseText(response daemon.Response) string {
	if response.Data == "" {
		return response.Message
	}
	return response.Message + "\n" + response.Data
}

func friendlyError(prefix string, err error) string {
	if err == nil {
		return prefix
	}
	return prefix + "\n\nDetail teknis:\n" + err.Error()
}
