package menu

import (
	"fmt"
	"os"
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

const (
	latencyRefreshInterval = 10 * time.Second
	latencyCheckTimeout    = 3 * time.Second
)

type latencyTickMsg time.Time

type homeRefreshMsg struct {
	profiles []profile.Profile
	active   *runtime.State
	err      error
}

type latencyResultsMsg struct {
	results   []latency.Result
	checkedAt time.Time
}

type model struct {
	opts             Options
	paths            config.Paths
	cfg              config.Config
	stage            stage
	cursor           int
	items            []menuItem
	profiles         []profile.Profile
	input            textinput.Model
	pending          profile.Profile
	message          string
	errMessage       string
	active           *runtime.State
	latencyResults   map[string]latency.Result
	latencyChecking  bool
	latencyCheckedAt time.Time
	homeErr          string
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	choiceStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	activeStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
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
		opts:           opts,
		paths:          paths,
		cfg:            cfg,
		stage:          stageHome,
		latencyResults: make(map[string]latency.Result),
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
	return tea.Batch(textinput.Blink, latencyTick(), refreshHome(m.paths))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case latencyTickMsg:
		return m, tea.Batch(latencyTick(), refreshHome(m.paths))
	case homeRefreshMsg:
		m.profiles = msg.profiles
		m.active = msg.active
		m.homeErr = ""
		if msg.err != nil {
			m.homeErr = msg.err.Error()
		}
		m.clampCursor()
		if len(m.profiles) == 0 {
			m.latencyChecking = false
			m.latencyResults = make(map[string]latency.Result)
			return m, nil
		}
		if m.latencyChecking {
			return m, nil
		}
		m.latencyChecking = true
		return m, checkLatencies(m.profiles)
	case latencyResultsMsg:
		m.latencyChecking = false
		m.latencyCheckedAt = msg.checkedAt
		m.latencyResults = make(map[string]latency.Result, len(msg.results))
		for _, result := range msg.results {
			m.latencyResults[result.Profile.ID] = result
		}
		return m, nil
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
		return m.withMessage(fmt.Sprintf("Profile tersimpan.\n\nNama: %s\nProtocol: %s\nServer: %s:%d", imported.Name, imported.Protocol, imported.Address, imported.Port)), refreshHome(m.paths)
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
		return m.withMessage(responseText(response)), refreshHome(m.paths)
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
		results := latency.CheckAllConcurrent(items, latencyCheckTimeout)
		return m.withMessage("Test Profile\n\n" + latency.FormatResults(results)), nil
	case actionDisconnect:
		response, err := daemon.RequestDaemon(m.opts.Socket, daemon.Request{Action: "disconnect"})
		if err != nil {
			return m.withMessage(friendlyError("Gagal disconnect. Service latar belakang mungkin belum aktif.", err)), nil
		}
		return m.withMessage(responseText(response)), refreshHome(m.paths)
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
	b.WriteString(m.viewConnectionSummary() + "\n\n")
	b.WriteString(m.viewProfileLatencyTable() + "\n\n")
	b.WriteString(headerStyle.Render("Menu") + "\n")
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
	b.WriteString(mutedStyle.Render("Enter untuk connect. Esc untuk kembali. Latency otomatis diperbarui setiap 10 detik.") + "\n\n")
	for i, item := range m.profiles {
		cursor := "  "
		style := choiceStyle
		if i == m.cursor {
			cursor = "> "
			style = cursorStyle
		}
		active := " "
		if m.isActiveProfile(item) {
			active = "*"
		}
		line := fmt.Sprintf("%s%s %-18s %-8s %-28s %s",
			cursor,
			active,
			truncate(item.Name, 18),
			item.Protocol,
			truncate(fmt.Sprintf("%s:%d", item.Address, item.Port), 28),
			m.latencyText(item),
		)
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

func (m model) viewConnectionSummary() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Status") + "\n")
	if m.active == nil {
		b.WriteString("Koneksi: " + mutedStyle.Render("Belum terhubung") + "\n")
	} else {
		b.WriteString("Koneksi: " + successStyle.Render("Terhubung") + "\n")
		b.WriteString("Aktif:   " + activeStyle.Render(fmt.Sprintf("%s (%s)", m.active.ProfileName, m.active.ProfileID)) + "\n")
		if item, ok := m.profileByID(m.active.ProfileID); ok {
			b.WriteString(fmt.Sprintf("Server:  %s:%d\n", item.Address, item.Port))
		}
		tunLine := strings.TrimSpace(fmt.Sprintf("%s %s", m.active.TunName, m.active.TunCIDR))
		if tunLine != "" {
			b.WriteString("TUN:     " + tunLine + "\n")
		}
		if m.active.DNS != nil && m.active.DNS.Enabled {
			b.WriteString("DNS:     " + strings.Join(m.active.DNS.Servers, ", ") + "\n")
		}
	}
	if m.homeErr != "" {
		b.WriteString(warnStyle.Render("Peringatan: "+m.homeErr) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) viewProfileLatencyTable() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Profile & Latency") + "\n")
	if len(m.profiles) == 0 {
		b.WriteString(mutedStyle.Render("Belum ada profile. Pilih Tambah Profile dulu.") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	status := "auto refresh 10 detik"
	if m.latencyChecking {
		status += " | checking"
	}
	if !m.latencyCheckedAt.IsZero() {
		status += " | update " + m.latencyCheckedAt.Format("15:04:05")
	}
	b.WriteString(mutedStyle.Render(status) + "\n")
	b.WriteString(mutedStyle.Render("  NAME               PROTO    SERVER                       LATENCY") + "\n")
	for _, item := range m.profiles {
		marker := " "
		style := choiceStyle
		if m.isActiveProfile(item) {
			marker = "*"
			style = activeStyle
		}
		line := fmt.Sprintf("%s %-18s %-8s %-28s %s",
			marker,
			truncate(item.Name, 18),
			item.Protocol,
			truncate(fmt.Sprintf("%s:%d", item.Address, item.Port), 28),
			m.latencyText(item),
		)
		b.WriteString(style.Render(line) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) latencyText(item profile.Profile) string {
	result, ok := m.latencyResults[item.ID]
	if !ok {
		if m.latencyChecking {
			return mutedStyle.Render("checking")
		}
		return mutedStyle.Render("pending")
	}
	text := latency.ResultText(result)
	switch result.Status {
	case latency.StatusOK:
		return successStyle.Render(text)
	case latency.StatusTimeout, latency.StatusFailed:
		return errorStyle.Render(text)
	default:
		return mutedStyle.Render(text)
	}
}

func (m model) isActiveProfile(item profile.Profile) bool {
	return m.active != nil && m.active.ProfileID == item.ID
}

func (m model) profileByID(id string) (profile.Profile, bool) {
	for _, item := range m.profiles {
		if item.ID == id {
			return item, true
		}
	}
	return profile.Profile{}, false
}

func (m *model) clampCursor() {
	if m.stage == stageHome {
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
		}
	} else if m.stage == stageSelectProfile {
		if m.cursor >= len(m.profiles) {
			m.cursor = len(m.profiles) - 1
		}
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func latencyTick() tea.Cmd {
	return tea.Tick(latencyRefreshInterval, func(t time.Time) tea.Msg {
		return latencyTickMsg(t)
	})
}

func refreshHome(paths config.Paths) tea.Cmd {
	return func() tea.Msg {
		items, profileErr := profile.LoadAll(paths.ProfilesFile)
		active, runtimeErr := readActiveRuntime(paths)
		return homeRefreshMsg{
			profiles: items,
			active:   active,
			err:      combineErrors(profileErr, runtimeErr),
		}
	}
}

func checkLatencies(items []profile.Profile) tea.Cmd {
	return func() tea.Msg {
		return latencyResultsMsg{
			results:   latency.CheckAllConcurrent(items, latencyCheckTimeout),
			checkedAt: time.Now(),
		}
	}
}

func readActiveRuntime(paths config.Paths) (*runtime.State, error) {
	if _, err := os.Stat(paths.RuntimeFile); os.IsNotExist(err) {
		return nil, nil
	}
	state, err := runtime.LoadState(paths.RuntimeFile)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func combineErrors(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "."
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
