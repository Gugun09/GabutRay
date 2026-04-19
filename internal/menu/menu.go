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
	defaultViewWidth       = 100
	defaultViewHeight      = 32
	wideLayoutMinWidth     = 104
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
	width            int
	height           int
}

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	brandStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	choiceStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cursorStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	activeStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	headerStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	panelStyle        = lipgloss.NewStyle().Padding(1, 2)
	cardStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(1, 2)
	activeCardStyle   = cardStyle.Copy().BorderForeground(lipgloss.Color("42"))
	selectedItemStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("36")).Padding(0, 1)
	footerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).PaddingTop(1)
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
		width:          defaultViewWidth,
		height:         defaultViewHeight,
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
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
	width := m.contentWidth()
	var body string
	if width >= wideLayoutMinWidth {
		sideWidth := 34
		gap := 2
		mainWidth := width - sideWidth - gap
		left := m.viewProfileLatencyCard(mainWidth)
		right := lipgloss.JoinVertical(lipgloss.Left,
			m.viewStatusCard(sideWidth),
			m.viewActionMenuCard(sideWidth),
		)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left,
			m.viewProfileLatencyCard(width),
			m.viewStatusCard(width),
			m.viewActionMenuCard(width),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewTitleBar(width),
		body,
		m.viewFooter(width),
	)
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

func (m model) viewTitleBar(width int) string {
	status := "DISCONNECTED"
	statusStyle := mutedStyle
	if m.active != nil {
		status = "CONNECTED"
		statusStyle = successStyle
	}
	left := brandStyle.Render("GabutRay") + " " + mutedStyle.Render("Linux TUN CLI")
	right := statusStyle.Render(status)
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

func (m model) viewStatusCard(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Status") + "\n\n")
	if m.active == nil {
		b.WriteString(mutedStyle.Render("Koneksi") + "\n")
		b.WriteString(errorStyle.Render("DISCONNECTED") + "\n")
	} else {
		b.WriteString(mutedStyle.Render("Koneksi") + "\n")
		b.WriteString(successStyle.Render("CONNECTED") + "\n\n")
		b.WriteString(mutedStyle.Render("Profile") + "\n")
		b.WriteString(activeStyle.Render(truncate(m.active.ProfileName, cardContentWidth(width))) + "\n")
		if item, ok := m.profileByID(m.active.ProfileID); ok {
			b.WriteString("\n" + mutedStyle.Render("Server") + "\n")
			b.WriteString(truncate(fmt.Sprintf("%s:%d", item.Address, item.Port), cardContentWidth(width)) + "\n")
		}
		tunLine := strings.TrimSpace(fmt.Sprintf("%s %s", m.active.TunName, m.active.TunCIDR))
		if tunLine != "" {
			b.WriteString("\n" + mutedStyle.Render("TUN") + "\n")
			b.WriteString(truncate(tunLine, cardContentWidth(width)) + "\n")
		}
		if m.active.DNS != nil && m.active.DNS.Enabled {
			b.WriteString("\n" + mutedStyle.Render("DNS") + "\n")
			b.WriteString(truncate(strings.Join(m.active.DNS.Servers, ", "), cardContentWidth(width)) + "\n")
		}
	}
	if m.homeErr != "" {
		b.WriteString("\n" + warnStyle.Render(truncate("Peringatan: "+m.homeErr, cardContentWidth(width))) + "\n")
	}
	return m.renderCard(width, strings.TrimRight(b.String(), "\n"), false)
}

func (m model) viewProfileLatencyCard(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Profile & Latency") + "\n")
	if len(m.profiles) == 0 {
		b.WriteString(mutedStyle.Render("Belum ada profile. Pilih Tambah Profile dulu.") + "\n")
		return m.renderCard(width, strings.TrimRight(b.String(), "\n"), false)
	}
	status := "refresh 10 detik"
	if m.latencyChecking {
		status += " | checking"
	}
	if !m.latencyCheckedAt.IsZero() {
		status += " | update " + m.latencyCheckedAt.Format("15:04:05")
	}
	b.WriteString(mutedStyle.Render(status) + "\n")
	b.WriteString("\n")
	contentWidth := cardContentWidth(width)
	stateW, nameW, protoW, latencyW, serverW := tableColumns(contentWidth)
	stateTitle := "STATE"
	if stateW <= 1 {
		stateTitle = "A"
	}
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
		stateW, stateTitle,
		nameW, "NAME",
		protoW, "PROTO",
		serverW, "SERVER",
		latencyW, "LATENCY",
	)
	b.WriteString(mutedStyle.Render(truncate(header, contentWidth)) + "\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("-", clampInt(contentWidth, 8, 120))) + "\n")
	for _, item := range m.profiles {
		state := ""
		style := choiceStyle
		if m.isActiveProfile(item) {
			state = "ACTIVE"
			if stateW <= 1 {
				state = "*"
			}
			style = activeStyle
		}
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s %s",
			stateW, state,
			nameW, truncate(item.Name, nameW),
			protoW, truncate(string(item.Protocol), protoW),
			serverW, truncate(fmt.Sprintf("%s:%d", item.Address, item.Port), serverW),
			m.latencyText(item),
		)
		b.WriteString(style.Render(line) + "\n")
	}
	return m.renderCard(width, strings.TrimRight(b.String(), "\n"), true)
}

func (m model) viewActionMenuCard(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Menu") + "\n\n")
	for i, item := range m.items {
		label := truncate(item.title, maxInt(8, cardContentWidth(width)-4))
		if i == m.cursor {
			b.WriteString(selectedItemStyle.Render("> "+label) + "\n")
			b.WriteString(mutedStyle.Render("  "+truncate(item.desc, maxInt(8, cardContentWidth(width)-2))) + "\n")
			continue
		}
		b.WriteString(choiceStyle.Render("  "+label) + "\n")
	}
	return m.renderCard(width, strings.TrimRight(b.String(), "\n"), false)
}

func (m model) viewFooter(width int) string {
	text := "Up/Down navigate  Enter select  Esc back  q quit"
	return footerStyle.Width(maxInt(1, width)).Render(truncate(text, width))
}

func (m model) renderCard(width int, body string, active bool) string {
	style := cardStyle
	if active {
		style = activeCardStyle
	}
	contentWidth := cardContentWidth(width)
	return style.Width(contentWidth).Render(body)
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

func (m model) contentWidth() int {
	if m.width <= 0 {
		return defaultViewWidth
	}
	return clampInt(m.width-4, 36, 140)
}

func cardContentWidth(width int) int {
	return maxInt(16, width-12)
}

func tableColumns(width int) (stateW, nameW, protoW, latencyW, serverW int) {
	if width < 48 {
		stateW = 1
		protoW = 5
		latencyW = 8
		nameW = clampInt(width/3, 8, 12)
		serverW = width - stateW - nameW - protoW - latencyW - 4
		if serverW < 8 {
			nameW = maxInt(6, nameW-(8-serverW))
			serverW = width - stateW - nameW - protoW - latencyW - 4
		}
		serverW = maxInt(8, serverW)
		return stateW, nameW, protoW, latencyW, serverW
	}
	stateW = 6
	protoW = 6
	latencyW = 10
	nameW = clampInt(width/4, 10, 22)
	serverW = width - stateW - nameW - protoW - latencyW - 4
	if serverW < 14 {
		deficit := 14 - serverW
		nameW = maxInt(8, nameW-deficit)
		serverW = width - stateW - nameW - protoW - latencyW - 4
	}
	serverW = maxInt(12, serverW)
	return stateW, nameW, protoW, latencyW, serverW
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

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
