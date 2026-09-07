package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Session es la ficha que escriben agent-event y agent-board-sync en
// $AGENT_BOARD_DIR/sessions/<agent>-<id>.json.
type Session struct {
	Agent      string  `json:"agent"`
	SessionID  string  `json:"session_id"`
	Name       string  `json:"name"`
	Title      string  `json:"title"`
	TitleSrc   string  `json:"title_source"`
	Project    string  `json:"project"`
	Cwd        string  `json:"cwd"`
	TmuxTarget string  `json:"tmux_target"`
	Type       string  `json:"type"`
	TypeSource string  `json:"type_source"`
	State      string  `json:"state"`
	WaitingFor *string `json:"waiting_for"`
	Goal       string  `json:"goal"`
	LastMsg    string  `json:"last_message"`
	Model      string  `json:"model"`
	Kind       string  `json:"kind"`
	LastEvent  string  `json:"last_event"`
	Subagents  int     `json:"subagents"`
	Turns      int     `json:"turns"`
	Started    int64   `json:"started"`
	Updated    int64   `json:"updated"`
	Pid        *int    `json:"pid"`
	file       string
	win        *window
}

var stateOrder = map[string]int{"waiting": 0, "error": 1, "working": 2, "idle": 3, "done": 4}
var types = []string{"impl", "plan", "fix", "review", "ops", "doc"}
var titleRe = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)/(impl|plan|fix|review|ops|doc): *(.+)$`)

func stateDir(cfg *Config) string {
	if d := os.Getenv("AGENT_BOARD_DIR"); d != "" {
		return d
	}
	if cfg.UI.StateDir != "" {
		return os.ExpandEnv(strings.Replace(cfg.UI.StateDir, "~", os.Getenv("HOME"), 1))
	}
	return filepath.Join(os.Getenv("HOME"), ".local/state/agent-board")
}

func loadSessions(dir string) []Session {
	files, _ := filepath.Glob(filepath.Join(dir, "sessions", "*.json"))
	var ss []Session
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(b, &s) != nil || s.SessionID == "" {
			continue
		}
		s.file = f
		if s.State == "" {
			s.State = "idle"
		}
		if s.Project == "" {
			s.Project = filepath.Base(s.Cwd)
		}
		ss = append(ss, s)
	}
	return ss
}

// shortName: la descripción sin el prefijo proyecto/tipo:.
func shortName(s *Session) string {
	name := s.Title
	if name == "" {
		name = s.Name
	}
	if name == "" && len(s.SessionID) >= 8 {
		name = s.SessionID[:8]
	}
	if idx := strings.Index(name, ": "); idx > 0 && strings.Contains(name[:idx], "/") {
		name = name[idx+2:]
	}
	return name
}

type row struct {
	header  string
	count   int
	session *Session
}

// buildRows agrupa por proyecto (los que tienen alguien esperando, primero) y
// dentro por estado y actividad.
func buildRows(sessions []Session, showDone bool, filter string) []row {
	var vis []Session
	for _, s := range sessions {
		if s.State == "done" && !showDone {
			continue
		}
		if filter != "" {
			hay := strings.ToLower(s.Title + " " + s.Name + " " + s.Project + " " + s.Type + " " + s.Goal + " " + s.Agent)
			if !strings.Contains(hay, strings.ToLower(filter)) {
				continue
			}
		}
		vis = append(vis, s)
	}
	prio := map[string]int{}
	byProj := map[string][]Session{}
	for _, s := range vis {
		p := stateOrder[s.State]
		if cur, ok := prio[s.Project]; !ok || p < cur {
			prio[s.Project] = p
		}
		byProj[s.Project] = append(byProj[s.Project], s)
	}
	projects := make([]string, 0, len(byProj))
	for p := range byProj {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		if prio[projects[i]] != prio[projects[j]] {
			return prio[projects[i]] < prio[projects[j]]
		}
		return projects[i] < projects[j]
	})
	var rows []row
	for _, p := range projects {
		list := byProj[p]
		sort.Slice(list, func(i, j int) bool {
			if stateOrder[list[i].State] != stateOrder[list[j].State] {
				return stateOrder[list[i].State] < stateOrder[list[j].State]
			}
			return list[i].Updated > list[j].Updated
		})
		rows = append(rows, row{header: p, count: len(list)})
		for i := range list {
			s := list[i]
			rows = append(rows, row{session: &s})
		}
	}
	return rows
}

// ── icono de proyecto: config > detección por ficheros del directorio ───────
var projectIconCache = map[string]string{}

func projectIcon(cfg *Config, project, cwd string) string {
	if pc, ok := cfg.Projects[project]; ok && pc.Icon != "" {
		return pc.Icon
	}
	if v, ok := projectIconCache[cwd]; ok {
		return v
	}
	root := cwd
	if out, err := runOut("git", "-C", cwd, "rev-parse", "--show-toplevel"); err == nil && out != "" {
		root = out
	}
	kind := "project"
	if cwd == os.Getenv("HOME") {
		kind = "home"
	} else {
		checks := []struct{ file, kind string }{
			{"go.mod", "go"}, {"Cargo.toml", "rust"}, {"pyproject.toml", "python"}, {"requirements.txt", "python"},
			{"astro.config.mjs", "astro"}, {"astro.config.ts", "astro"}, {"package.json", "node"}, {".git", "git"},
		}
		for _, ch := range checks {
			if _, err := os.Stat(filepath.Join(root, ch.file)); err == nil {
				kind = ch.kind
				break
			}
		}
	}
	v := ic(kind)
	if v == "" {
		v = ic("project")
	}
	projectIconCache[cwd] = v
	return v
}

func projectLabel(cfg *Config, project string) string {
	if pc, ok := cfg.Projects[project]; ok && pc.Label != "" {
		return pc.Label
	}
	return project
}

func projectColor(cfg *Config, project string) string {
	if pc, ok := cfg.Projects[project]; ok && pc.Color != "" {
		return pc.Color
	}
	return "accent"
}
