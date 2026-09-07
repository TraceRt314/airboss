package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	cfg      *Config
	dir      string
	sessions []Session
	rows     []row
	cursor   int
	showDone bool
	filter   string
	mode     string // "", filter, new, rename, help
	input    string
	width    int
	height   int
	status   string
	statusAt time.Time
	lastSync time.Time
	syncing  bool
	frame    int
	spin     []string
}

type tickMsg time.Time
type spinMsg time.Time
type syncMsg struct{ err error }
type execMsg struct {
	err error
	ok  string
}

func newModel(cfg *Config) model {
	sp, ok := spinners[cfg.UI.Spinner]
	if !ok {
		sp = spinners["braille"]
	}
	m := model{cfg: cfg, dir: stateDir(cfg), width: 100, height: 30, spin: sp}
	m.reload()
	return m
}

func (m *model) reload() {
	ss := loadSessions(m.dir)
	wins := listWindows()
	for i := range ss {
		if ss[i].State != "done" {
			ss[i].win = resolveWindow(&ss[i], wins)
		}
	}
	m.sessions = ss
	m.buildRows()
}

func (m *model) buildRows() {
	m.rows = buildRows(m.sessions, m.showDone, m.filter)
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.skipHeader(1)
}

func (m *model) skipHeader(dir int) {
	for m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].session == nil {
		m.cursor += dir
	}
	if m.cursor < 0 {
		m.cursor = 0
		m.skipHeader(1)
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
		if m.cursor > 0 {
			m.skipHeader(-1)
		}
	}
}

func (m model) selected() *Session {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor].session
	}
	return nil
}

func (m model) anyWorking() bool {
	for _, s := range m.sessions {
		if s.State == "working" {
			return true
		}
	}
	return m.syncing
}

func (m *model) setStatus(s string) { m.status, m.statusAt = s, time.Now() }

func (m model) tick() tea.Cmd {
	return tea.Tick(time.Duration(m.cfg.UI.TickSeconds)*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func spin() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return spinMsg(t) })
}

func syncBin() string { return filepath.Join(os.Getenv("HOME"), ".local/bin/agent-board-sync") }

func syncCmd() tea.Cmd {
	if os.Getenv("TORRE_NO_SYNC") != "" { // demos y capturas: solo las fichas del directorio
		return func() tea.Msg { return syncMsg{} }
	}
	return func() tea.Msg { return syncMsg{err: exec.Command(syncBin()).Run()} }
}

func (m model) Init() tea.Cmd { return tea.Batch(m.tick(), spin(), syncCmd()) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = msg.Width
		}
		if msg.Height > 10 {
			m.height = msg.Height
		}
		return m, nil
	case tickMsg:
		m.reload()
		var cmd tea.Cmd
		if !m.syncing && time.Since(m.lastSync) > 8*time.Second {
			m.syncing = true
			cmd = syncCmd()
		}
		return m, tea.Batch(m.tick(), cmd)
	case spinMsg:
		if m.anyWorking() {
			m.frame++
		}
		if m.status != "" && time.Since(m.statusAt) > 6*time.Second {
			m.status = ""
		}
		return m, spin()
	case syncMsg:
		m.syncing = false
		m.lastSync = time.Now()
		if msg.err != nil {
			m.setStatus("sync: " + msg.err.Error())
		}
		m.reload()
		return m, nil
	case execMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		} else if msg.ok != "" {
			m.setStatus(msg.ok)
		}
		m.reload()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == "help" {
		m.mode = ""
		return m, nil
	}
	if m.mode == "filter" || m.mode == "new" || m.mode == "rename" {
		switch k.Type {
		case tea.KeyEsc:
			if m.mode == "filter" {
				m.filter = ""
			}
			m.mode, m.input = "", ""
			m.buildRows()
			return m, nil
		case tea.KeyEnter:
			switch m.mode {
			case "new":
				return m.launchNew()
			case "rename":
				m.rename()
				return m, nil
			}
			m.mode = ""
			return m, nil
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.input)
				m.input = m.input[:len(m.input)-size]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.input += k.String()
		}
		if m.mode == "filter" {
			m.filter = m.input
			m.buildRows()
		}
		return m, nil
	}
	s := m.selected()
	switch k.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
			m.skipHeader(1)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.skipHeader(-1)
		}
	case "g", "home":
		m.cursor = 0
		m.skipHeader(1)
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.skipHeader(-1)
	case "d":
		m.showDone = !m.showDone
		m.buildRows()
	case "s":
		m.syncing = true
		m.setStatus(T("syncing"))
		return m, syncCmd()
	case "/":
		m.mode, m.input = "filter", m.filter
	case "n":
		m.mode = "new"
		proj := "project"
		if s != nil {
			proj = s.Project
		}
		m.input = proj + "/impl: "
	case "r":
		if s != nil {
			m.mode = "rename"
			m.input = s.Title
			if m.input == "" {
				m.input = s.Project + "/" + s.Type + ": " + shortName(s)
			}
		}
	case "enter", "l", "right":
		if s != nil {
			return m, m.focus(s)
		}
	case "a":
		if s != nil {
			return m, m.attach(s)
		}
	case "t":
		if s != nil {
			m.cycleType(s)
		}
	case "x":
		if s != nil {
			os.Remove(s.file)
			m.setStatus(T("archived") + ": " + shortName(s))
			m.reload()
		}
	case "?":
		m.mode = "help"
	}
	return m, nil
}

func agentEvent(args ...string) error {
	return exec.Command(filepath.Join(os.Getenv("HOME"), ".local/bin/agent-event"), args...).Run()
}

func (m *model) cycleType(s *Session) {
	idx := 0
	for i, t := range types {
		if t == s.Type {
			idx = (i + 1) % len(types)
		}
	}
	patch := map[string]any{"type": types[idx], "type_source": "user"}
	if mm := titleRe.FindStringSubmatch(s.Title); mm != nil {
		patch["title"] = mm[1] + "/" + types[idx] + ": " + mm[3]
		patch["title_source"] = "user"
	}
	patchFile(s.file, patch)
	agentEvent("apply-title", s.file)
	m.setStatus(fmt.Sprintf("%s → %s", shortName(s), types[idx]))
	m.reload()
}

func (m *model) rename() {
	s := m.selected()
	title := strings.TrimSpace(m.input)
	m.mode, m.input = "", ""
	if s == nil || title == "" {
		return
	}
	patch := map[string]any{"title": title, "title_source": "user", "title_pending": 0}
	if mm := titleRe.FindStringSubmatch(title); mm != nil {
		patch["project"] = mm[1]
		patch["type"] = mm[2]
		patch["type_source"] = "name"
	}
	patchFile(s.file, patch)
	agentEvent("apply-title", s.file)
	m.setStatus(T("renamed") + ": " + title)
	m.reload()
}

func patchFile(file string, patch map[string]any) {
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	var obj map[string]any
	if json.Unmarshal(b, &obj) != nil {
		return
	}
	for k, v := range patch {
		obj[k] = v
	}
	if out, err := json.MarshalIndent(obj, "", "  "); err == nil {
		os.WriteFile(file, out, 0o644)
	}
}

func (m model) focus(s *Session) tea.Cmd {
	win, target := s.win, s.TmuxTarget
	if win == nil && target == "" {
		return func() tea.Msg {
			if s.Pid == nil {
				return execMsg{err: fmt.Errorf(T("no_term"), shortName(s))}
			}
			return execMsg{err: fmt.Errorf(T("no_win"), shortName(s))}
		}
	}
	return func() tea.Msg {
		if win != nil {
			if err := focusWindow(win); err != nil {
				return execMsg{err: err}
			}
		}
		focusTmux(target)
		if win != nil {
			return execMsg{ok: ic("goal") + " " + winLabel(win)}
		}
		return execMsg{ok: ic("goal") + " tmux " + target}
	}
}

func (m model) attach(s *Session) tea.Cmd {
	var cmd *exec.Cmd
	switch {
	case s.Agent == "claude" && s.Kind == "background":
		cmd = exec.Command("claude", "attach", s.SessionID)
	case s.Agent == "claude" && s.State == "done":
		cmd = exec.Command("claude", "--resume", s.SessionID)
	case s.Agent == "codex" && (s.State == "done" || s.Pid == nil):
		cmd = exec.Command("codex", "resume", s.SessionID)
	default:
		return m.focus(s)
	}
	cmd.Dir = s.Cwd
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return execMsg{err: err} })
}

func (m model) launchNew() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.input)
	m.mode, m.input = "", ""
	if name == "" {
		return m, nil
	}
	proj, _, _ := strings.Cut(name, "/")
	cwd := os.Getenv("HOME")
	for _, s := range m.sessions {
		if s.Project == proj {
			cwd = s.Cwd
			break
		}
	}
	if os.Getenv("TMUX") == "" {
		m.setStatus(T("need_tmux"))
		return m, nil
	}
	// claude --name pone el título en formato Torre desde el primer segundo
	launch := "claude --name " + shellQuote(name)
	if _, err := exec.LookPath("cl"); err == nil {
		typ, desc := "impl", name
		if mm := titleRe.FindStringSubmatch(name); mm != nil {
			typ, desc = mm[2], mm[3]
		}
		launch = "cl " + typ + " " + shellQuote(desc)
	}
	if err := exec.Command("tmux", "new-window", "-c", cwd, "-n", proj, launch).Run(); err != nil {
		m.setStatus("tmux: " + err.Error())
	} else {
		m.setStatus(T("launched") + " " + name)
	}
	return m, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// ── helpers de texto ────────────────────────────────────────────────────────
func trunc(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if n <= 1 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func rel(ts int64) string {
	if ts == 0 {
		return ""
	}
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return T("now")
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
	case d < 48*time.Hour:
		return T("yesterday")
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func wrap(s string, width int) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, w := range strings.Fields(para) {
			if cur == "" {
				cur = w
			} else if lipgloss.Width(cur)+1+lipgloss.Width(w) <= width {
				cur += " " + w
			} else {
				lines = append(lines, cur)
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	return lines
}

func modelShort(m string) string {
	m = strings.TrimPrefix(m, "claude-")
	m = strings.ReplaceAll(m, "-sol", "")
	return m
}

func tilde(p string) string { return strings.Replace(p, os.Getenv("HOME"), "~", 1) }

func agentIcon(a string) string {
	if v := ic(a); v != "" {
		return v
	}
	return ic("app")
}

// ── vista ───────────────────────────────────────────────────────────────────
func (m model) View() string {
	cfg := m.cfg
	dim := lipgloss.NewStyle().Foreground(c("dark_foreground"))
	mut := lipgloss.NewStyle().Foreground(c("muted"))
	fg := lipgloss.NewStyle().Foreground(c("foreground"))
	lfg := lipgloss.NewStyle().Foreground(c("light_foreground"))
	acc := lipgloss.NewStyle().Foreground(c("accent")).Bold(true)
	selBg := lipgloss.NewStyle().Background(c("lighter_background"))
	key := lipgloss.NewStyle().Foreground(c("background")).Background(c("accent")).Bold(true)
	w := m.width
	if m.mode == "help" {
		return m.helpView()
	}

	// ── cabecera ──
	nAct, nWait, nWork := 0, 0, 0
	for _, s := range m.sessions {
		if s.State != "done" {
			nAct++
		}
		if s.State == "waiting" {
			nWait++
		}
		if s.State == "working" {
			nWork++
		}
	}
	left := acc.Render(" " + icf("app") + cfg.UI.Title)
	if themeName != "" {
		left += mut.Render("  " + icf("theme") + themeName)
	}
	chip := func(n int, word, icon string, col lipgloss.Color, bold bool) string {
		st := lipgloss.NewStyle().Foreground(col).Bold(bold)
		if n == 0 {
			st = dim
		}
		txt := fmt.Sprintf("%s%d %s", icf(icon), n, word)
		if ic("chip_l") != "" && n > 0 {
			bg := lipgloss.NewStyle().Foreground(c("background")).Background(col).Bold(bold)
			cap := lipgloss.NewStyle().Foreground(col)
			return cap.Render(ic("chip_l")) + bg.Render(txt) + cap.Render(ic("chip_r"))
		}
		return st.Render(txt)
	}
	right := chip(nAct, T("n_active"), "active", c("light_foreground"), false) + " " +
		chip(nWork, T("n_working"), "working", c("green"), false) + " " +
		chip(nWait, T("n_waiting"), "bell", c("red"), true)
	if m.filter != "" {
		right = lipgloss.NewStyle().Foreground(c("yellow")).Render(icf("filter")+m.filter) + dim.Render("   ") + right
	}
	syncTxt := "  "
	if m.syncing {
		syncTxt = acc.Render(m.spin[m.frame%len(m.spin)]) + " "
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - lipgloss.Width(syncTxt)
	if gap < 1 {
		gap = 1
	}
	var b strings.Builder
	b.WriteString("\n" + left + strings.Repeat(" ", gap) + right + syncTxt + "\n")
	b.WriteString(mut.Render(" "+strings.Repeat("─", w-2)) + "\n")

	// ── lista ──
	detailH := 0
	if cfg.UI.Details {
		detailH = cfg.UI.DetailLines + 2
	}
	listH := m.height - 4 - detailH - 2
	if listH < 4 {
		listH = 4
	}
	var lines []string
	selLine := 0
	for i, r := range m.rows {
		if r.session == nil {
			pc := lipgloss.NewStyle().Foreground(c(projectColor(cfg, r.header))).Bold(true)
			pcwd := ""
			for j := i + 1; j < len(m.rows) && m.rows[j].session != nil; j++ {
				pcwd = m.rows[j].session.Cwd
				break
			}
			title := pc.Render(ic("bar") + projectIcon(cfg, r.header, pcwd) + " " + projectLabel(cfg, r.header))
			cnt := dim.Render(fmt.Sprintf("%d", r.count))
			fill := w - 2 - lipgloss.Width(title) - lipgloss.Width(cnt) - 2
			if fill < 1 {
				fill = 1
			}
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, " "+title+" "+mut.Render(strings.Repeat(ic("rule"), fill))+" "+cnt)
			continue
		}
		s := r.session
		sel := i == m.cursor
		if sel {
			selLine = len(lines)
		}
		bgc := c("lighter_background")
		// con la fila seleccionada cada segmento lleva el fondo: lipgloss no lo
		// propaga a través de los resets de los estilos anidados
		S := func(st lipgloss.Style) lipgloss.Style {
			if sel {
				return st.Background(bgc)
			}
			return st
		}
		sp := func(n int) string {
			if n < 1 {
				n = 1
			}
			if sel {
				return selBg.Render(strings.Repeat(" ", n))
			}
			return strings.Repeat(" ", n)
		}
		sc := stateColor(s.State)
		dotTxt := ic(s.State)
		if s.State == "working" {
			dotTxt = m.spin[m.frame%len(m.spin)]
		}
		if dotTxt == "" {
			dotTxt = "●"
		}
		bar := sp(1)
		nameSt := S(lfg)
		if sel {
			bar = S(acc).Render(ic("sel"))
			nameSt = S(fg).Bold(true)
		}
		dot := S(lipgloss.NewStyle().Foreground(sc)).Render(dotTxt)
		st := S(lipgloss.NewStyle().Foreground(sc).Bold(s.State == "waiting" || s.State == "error")).Render(pad(T(s.State), 7))
		tyTxt := typeLabel(cfg, s.Type)
		if ti := typeIcon(cfg, s.Type); ti != "" {
			tyTxt = ti + " " + tyTxt
		}
		ty := S(lipgloss.NewStyle().Foreground(typeColor(cfg, s.Type))).Render(pad(tyTxt, 8))
		mid := ""
		if s.State == "waiting" && s.WaitingFor != nil {
			mid = S(lipgloss.NewStyle().Foreground(c("red"))).Render("  " + icf("waiting") + *s.WaitingFor)
		} else if s.LastEvent != "" && s.State == "working" {
			mid = S(dim).Render("  · " + s.LastEvent)
		}
		l1 := bar + sp(1) + dot + sp(1) + st + sp(1) + ty + sp(1) + nameSt.Render(trunc(shortName(s), cfg.UI.NameWidth)) + mid
		meta := agentIcon(s.Agent) + " " + s.Agent
		if ms := modelShort(s.Model); ms != "" {
			meta += " · " + icf("model") + ms
		}
		ts := s.Updated
		if ts == 0 {
			ts = s.Started
		}
		meta += " · " + icf("clock") + rel(ts)
		if s.Subagents > 0 {
			meta = fmt.Sprintf("%s%d · ", icf("sub"), s.Subagents) + meta
		}
		metaR := S(dim).Render(meta)
		line1 := l1 + sp(w-lipgloss.Width(l1)-lipgloss.Width(metaR)-1)
		line1 += metaR + sp(w-lipgloss.Width(line1)-lipgloss.Width(metaR))
		lines = append(lines, line1)
		if cfg.UI.ShowGoal {
			goal := s.Goal
			if goal == "" {
				goal = s.LastMsg
			}
			if goal == "" {
				goal = tilde(s.Cwd) + "  (" + T("no_goal_short") + ")"
			}
			where := ""
			if s.State != "done" {
				switch {
				case s.win != nil:
					where = icf("window") + "ws " + s.win.Workspace.Name + " · " + s.win.Class
				case s.TmuxTarget != "":
					where = icf("tmux") + "tmux " + s.TmuxTarget
				default:
					where = icf("nowindow") + T("no_window")
				}
			}
			whereR := S(mut).Render(where)
			l2 := bar + sp(19) + S(dim).Render(ic("goal")+" "+trunc(goal, w-24-lipgloss.Width(where)))
			line2 := l2 + sp(w-lipgloss.Width(l2)-lipgloss.Width(whereR)-1)
			line2 += whereR + sp(w-lipgloss.Width(line2)-lipgloss.Width(whereR))
			lines = append(lines, line2)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "", dim.Render("   "+T("empty")))
	}
	start := 0
	if selLine >= listH-1 {
		start = selLine - listH + 2
	}
	end := start + listH
	if end > len(lines) {
		end = len(lines)
	}
	for _, l := range lines[start:end] {
		b.WriteString(l + "\n")
	}
	for i := end - start; i < listH; i++ {
		b.WriteString("\n")
	}

	// ── detalle ──
	if cfg.UI.Details {
		iw := w - 6
		var det []string
		boxTitle := ""
		if s := m.selected(); s != nil {
			lbl := func(t string) string { return mut.Render(pad(t, 9)) }
			boxTitle = lipgloss.NewStyle().Foreground(typeColor(cfg, s.Type)).Bold(true).Render(icf(s.Type)+s.Project+"/"+s.Type) +
				mut.Render(": ") + lfg.Render(trunc(shortName(s), iw-30))
			if s.WaitingFor != nil && *s.WaitingFor != "" {
				det = append(det, lbl(T("waiting"))+lipgloss.NewStyle().Foreground(c("red")).Bold(true).Render(icf("waiting")+trunc(*s.WaitingFor, iw-12)))
			}
			goalLines := wrap(s.Goal, iw-9)
			if len(goalLines) == 0 {
				goalLines = []string{dim.Render(T("no_goal"))}
			}
			for i, g := range goalLines {
				if i == 2 {
					break
				}
				l := lbl("")
				if i == 0 {
					l = lbl(T("goal"))
				}
				det = append(det, l+fg.Render(g))
			}
			if s.LastMsg != "" {
				det = append(det, lbl(T("last"))+lfg.Render(trunc(s.LastMsg, iw-9)))
			}
			switch {
			case s.win != nil:
				det = append(det, lbl(T("window"))+lipgloss.NewStyle().Foreground(c("green")).Render(icf("window")+trunc(winLabel(s.win), iw-16))+dim.Render("   "+T("to_go")))
			case s.State == "done":
				det = append(det, lbl(T("window"))+dim.Render(T("ended")+"   "+T("attach_hint")))
			case s.TmuxTarget != "":
				det = append(det, lbl(T("window"))+lipgloss.NewStyle().Foreground(c("green")).Render(icf("tmux")+"tmux "+s.TmuxTarget)+dim.Render("   "+T("to_go")))
			default:
				det = append(det, lbl(T("window"))+lipgloss.NewStyle().Foreground(c("yellow")).Render(icf("nowindow")+T("not_found"))+dim.Render("   "+T("attach_hint")))
			}
			sid := s.SessionID
			if len(sid) > 8 {
				sid = sid[:8]
			}
			info := fmt.Sprintf("%s · %s · %d %s · %s %s · %s %s · %s %s", tilde(s.Cwd), sid, s.Turns, T("turns"), T("since"), rel(s.Started), T("type_by"), s.TypeSource, T("title_by"), s.TitleSrc)
			det = append(det, lbl("")+dim.Render(trunc(info, iw-9)))
		}
		tl, tr, bl, br, hz, vt := "╭", "╮", "╰", "╯", "─", "│"
		switch cfg.UI.Border {
		case "square":
			tl, tr, bl, br = "┌", "┐", "└", "┘"
		case "none":
			tl, tr, bl, br, hz, vt = " ", " ", " ", " ", " ", " "
		}
		head := mut.Render(" " + tl + hz)
		if boxTitle != "" {
			head += " " + boxTitle + " "
		}
		fill := w - 2 - lipgloss.Width(head)
		if fill < 0 {
			fill = 0
		}
		b.WriteString(head + mut.Render(strings.Repeat(hz, fill)+tr) + "\n")
		for i := 0; i < cfg.UI.DetailLines; i++ {
			l := ""
			if i < len(det) {
				l = det[i]
			}
			b.WriteString(mut.Render(" "+vt+" ") + pad(trunc(l, iw), iw) + mut.Render(" "+vt) + "\n")
		}
		b.WriteString(mut.Render(" "+bl+strings.Repeat(hz, w-4)+br) + "\n")
	}

	// ── pie ──
	hint := func(k, t string) string { return key.Render(" "+k+" ") + dim.Render(" "+t) }
	// tantas pistas como quepan (trunc por runas cortaría dentro de las secuencias ANSI)
	foot := " "
	for _, h := range []string{
		hint("↵", T("k_go")), hint("a", T("k_attach")), hint("n", T("k_new")), hint("r", T("k_rename")), hint("t", T("k_type")),
		hint("x", T("k_archive")), hint("d", T("k_done")), hint("/", T("k_filter")), hint("?", T("k_help")), hint("q", T("k_quit")),
	} {
		if lipgloss.Width(foot)+lipgloss.Width(h)+2 > w {
			break
		}
		foot += h + "  "
	}
	switch m.mode {
	case "filter":
		foot = " " + key.Render(" / ") + " " + fg.Render(m.input) + acc.Render("▏") + dim.Render("   "+T("filter_hint"))
	case "new":
		foot = " " + key.Render(" n ") + dim.Render(" "+T("new_hint")) + fg.Render(m.input) + acc.Render("▏") + dim.Render("   "+T("new_tail"))
	case "rename":
		foot = " " + key.Render(" r ") + dim.Render(" "+T("rename_hint")) + fg.Render(m.input) + acc.Render("▏") + dim.Render("   "+T("rename_tail"))
	}
	if m.status != "" && m.mode == "" {
		foot = " " + lipgloss.NewStyle().Foreground(c("yellow")).Render(trunc(m.status, w-3))
	}
	b.WriteString(foot)
	return b.String()
}

func (m model) helpView() string {
	acc := lipgloss.NewStyle().Foreground(c("accent")).Bold(true)
	mut := lipgloss.NewStyle().Foreground(c("muted"))
	fg := lipgloss.NewStyle().Foreground(c("foreground"))
	dim := lipgloss.NewStyle().Foreground(c("dark_foreground"))
	keys := []string{"h_nav", "h_go", "h_attach", "h_new", "h_rename", "h_type", "h_archive", "h_done", "h_sync", "h_filter", "h_quit"}
	var b strings.Builder
	b.WriteString("\n " + acc.Render(icf("help")+m.cfg.UI.Title+" · "+T("help_title")) + "\n")
	b.WriteString(mut.Render(" "+strings.Repeat("─", m.width-2)) + "\n\n")
	for _, k := range keys {
		txt := T(k)
		kpart, rest, _ := strings.Cut(txt, "  ")
		b.WriteString("   " + acc.Render(pad(kpart, 12)) + fg.Render(rest) + "\n")
	}
	b.WriteString("\n " + dim.Render(configPath()) + "\n")
	b.WriteString(" " + dim.Render(T("help_close")) + "\n")
	return b.String()
}
