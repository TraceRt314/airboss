package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is ~/.config/airboss/config.toml. Everything has a default: with no
// file, airboss follows the Omarchy theme if present and uses Nerd Font icons.
type Config struct {
	Theme    ThemeConfig              `toml:"theme"`
	Icons    IconConfig               `toml:"icons"`
	UI       UIConfig                 `toml:"ui"`
	Notify   NotifyConfig             `toml:"notify"`
	Features FeaturesConfig           `toml:"features"`
	Types    map[string]TypeConfig    `toml:"types"`
	Projects map[string]ProjectConfig `toml:"projects"`
}

type ThemeConfig struct {
	Source string            `toml:"source"` // auto | omarchy | builtin
	Name   string            `toml:"name"`   // builtin theme (see themes/)
	Colors map[string]string `toml:"colors"` // overrides: accent = "#ff79c6"
}

type IconConfig struct {
	Set      string            `toml:"set"` // nerd | unicode | ascii
	Override map[string]string `toml:"override"`
}

type UIConfig struct {
	Title       string `toml:"title"`
	Lang        string `toml:"lang"` // es | en
	ShowGoal    bool   `toml:"show_goal"`
	Details     bool   `toml:"details"`
	DetailLines int    `toml:"detail_lines"`
	Border      string `toml:"border"` // rounded | square | none
	TickSeconds int    `toml:"tick_seconds"`
	NameWidth   int    `toml:"name_width"`
	Spinner     string `toml:"spinner"` // braille | dots | line | none
	StateDir    string `toml:"state_dir"`
}

// NotifyConfig decides which transitions are announced and where. The scripts
// read it through `airboss-tui config`, so there is one source of truth.
type NotifyConfig struct {
	Enabled  bool           `toml:"enabled"`
	Sinks    []string       `toml:"sinks"` // desktop | ntfy | telegram | command
	Events   NotifyEvents   `toml:"events"`
	Ntfy     NtfyConfig     `toml:"ntfy"`
	Telegram TelegramConfig `toml:"telegram"`
	Command  string         `toml:"command"` // run with <urgency> <icon> <title> <body>
}

type NotifyEvents struct {
	Waiting     bool `toml:"waiting"`      // the agent needs an answer from you
	Error       bool `toml:"error"`        // the agent errored
	TurnDone    bool `toml:"turn_done"`    // a turn ended and the agent went idle
	SessionDone bool `toml:"session_done"` // the session closed
}

type NtfyConfig struct {
	URL   string `toml:"url"`   // server, default https://ntfy.sh
	Topic string `toml:"topic"` // required to enable the sink
	Token string `toml:"token"` // literal, env:VAR or file:/path
}

type TelegramConfig struct {
	Token  string `toml:"token"`   // literal, env:VAR or file:/path
	ChatID string `toml:"chat_id"` // required to enable the sink
}

// FeaturesConfig turns whole parts of airboss off. Everything is on by default.
type FeaturesConfig struct {
	Classifier  bool `toml:"classifier"`   // name untitled sessions with a small model
	WindowFocus bool `toml:"window_focus"` // resolve and raise terminal windows
	ApplyTitle  bool `toml:"apply_title"`  // push the canonical title back into the CLI
	Sync        bool `toml:"sync"`         // the periodic reconcile pass
}

type TypeConfig struct {
	Icon  string `toml:"icon"`
	Color string `toml:"color"`
	Label string `toml:"label"`
}

type ProjectConfig struct {
	Icon  string `toml:"icon"`
	Color string `toml:"color"`
	Label string `toml:"label"`
}

func defaultConfig() Config {
	lang := "en"
	for _, v := range []string{os.Getenv("AIRBOSS_LANG"), os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if v != "" {
			if strings.HasPrefix(v, "es") {
				lang = "es"
			}
			break
		}
	}
	return Config{
		Theme: ThemeConfig{Source: "auto", Name: "catppuccin-mocha"},
		Icons: IconConfig{Set: "nerd"},
		UI: UIConfig{
			Title: "AIRBOSS", Lang: lang, ShowGoal: true, Details: true, DetailLines: 5,
			Border: "rounded", TickSeconds: 2, NameWidth: 44, Spinner: "braille",
		},
		Notify: NotifyConfig{
			Enabled: true,
			Sinks:   []string{"desktop"},
			Events:  NotifyEvents{Waiting: true, Error: true, TurnDone: true, SessionDone: true},
			Ntfy:    NtfyConfig{URL: "https://ntfy.sh"},
		},
		Features: FeaturesConfig{Classifier: true, WindowFocus: true, ApplyTitle: true, Sync: true},
	}
}

func configPath() string {
	if p := os.Getenv("AIRBOSS_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "airboss", "config.toml")
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		path = configPath()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		return cfg, err
	}
	if cfg.UI.DetailLines < 3 {
		cfg.UI.DetailLines = 3
	}
	if cfg.UI.TickSeconds < 1 {
		cfg.UI.TickSeconds = 1
	}
	if cfg.UI.NameWidth < 16 {
		cfg.UI.NameWidth = 16
	}
	return cfg, nil
}

// ── secrets ────────────────────────────────────────────────────────────────
// A token in config.toml can be the literal value, "env:VAR" or "file:/path",
// so a config file that lives in a dotfiles repo need not hold the secret.
// expandHome makes a leading ~/ usable in config values that become shell
// words: the shell does not tilde-expand the result of a variable expansion.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	return p
}

func resolveSecret(v string) string {
	switch {
	case strings.HasPrefix(v, "env:"):
		return os.Getenv(strings.TrimPrefix(v, "env:"))
	case strings.HasPrefix(v, "file:"):
		b, err := os.ReadFile(expandHome(strings.TrimPrefix(v, "file:")))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return v
}

// activeSinks drops sinks that are unknown or missing their required setting,
// so a half-written [notify.ntfy] never silently swallows notifications.
func (n NotifyConfig) activeSinks() []string {
	var out []string
	for _, s := range n.Sinks {
		switch strings.TrimSpace(strings.ToLower(s)) {
		case "desktop":
			out = append(out, "desktop")
		case "ntfy":
			if n.Ntfy.Topic != "" {
				out = append(out, "ntfy")
			}
		case "telegram":
			if n.Telegram.ChatID != "" && resolveSecret(n.Telegram.Token) != "" {
				out = append(out, "telegram")
			}
		case "command":
			if n.Command != "" {
				out = append(out, "command")
			}
		}
	}
	return out
}

// ── `airboss-tui config`: the resolved config as shell assignments ──────────
// The shell side of airboss (hooks, sync, notifier) evals this instead of
// parsing TOML, so config.toml stays the single source of truth.
func boolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func printShellConfig(cfg *Config) {
	kv := func(k, v string) { fmt.Printf("%s=%s\n", k, shellQuote(v)) }
	kv("AIRBOSS_LANG", cfg.UI.Lang)
	kv("AIRBOSS_STATE_DIR", stateDir(cfg))
	kv("AIRBOSS_NOTIFY_ENABLED", boolVal(cfg.Notify.Enabled))
	kv("AIRBOSS_NOTIFY_SINKS", strings.Join(cfg.Notify.activeSinks(), " "))
	kv("AIRBOSS_NOTIFY_WAITING", boolVal(cfg.Notify.Events.Waiting))
	kv("AIRBOSS_NOTIFY_ERROR", boolVal(cfg.Notify.Events.Error))
	kv("AIRBOSS_NOTIFY_TURN_DONE", boolVal(cfg.Notify.Events.TurnDone))
	kv("AIRBOSS_NOTIFY_SESSION_DONE", boolVal(cfg.Notify.Events.SessionDone))
	kv("AIRBOSS_NTFY_URL", strings.TrimSuffix(cfg.Notify.Ntfy.URL, "/"))
	kv("AIRBOSS_NTFY_TOPIC", cfg.Notify.Ntfy.Topic)
	kv("AIRBOSS_NTFY_TOKEN", resolveSecret(cfg.Notify.Ntfy.Token))
	kv("AIRBOSS_TELEGRAM_TOKEN", resolveSecret(cfg.Notify.Telegram.Token))
	kv("AIRBOSS_TELEGRAM_CHAT_ID", cfg.Notify.Telegram.ChatID)
	kv("AIRBOSS_NOTIFY_COMMAND", expandHome(cfg.Notify.Command))
	kv("AIRBOSS_FEATURE_CLASSIFIER", boolVal(cfg.Features.Classifier))
	kv("AIRBOSS_FEATURE_WINDOW_FOCUS", boolVal(cfg.Features.WindowFocus))
	kv("AIRBOSS_FEATURE_APPLY_TITLE", boolVal(cfg.Features.ApplyTitle))
	kv("AIRBOSS_FEATURE_SYNC", boolVal(cfg.Features.Sync))
}

// ── icons ───────────────────────────────────────────────────────────────────
// Three sets: nerd (Nerd Font v3, the kind seen on r/unixporn), unicode (any
// font) and ascii (terminals without unicode). Each key can be overridden in
// [icons.override].
var iconSets = map[string]map[string]string{
	"nerd": {
		"app": "\U000F16A3", "claude": "✳", "codex": "⬢",
		"waiting": "\uf0f3", "idle": "\uf186", "done": "\uf00c", "error": "\uf057", "working": "\uf013",
		"goal": "\uf178", "window": "\uf108", "tmux": "\uf120", "nowindow": "\uf05e", "sub": "\uf0e8", "model": "\uf2db", "clock": "\uf017",
		"impl": "\uf121", "plan": "\uf0eb", "fix": "\uf188", "review": "\uf002", "ops": "\uf0c2", "doc": "\uf0f6",
		"project": "\uf114", "home": "\uf015", "go": "\ue626", "node": "\ue718", "python": "\ue73c", "rust": "\ue7a8", "astro": "\uf135", "git": "\ue702",
		"theme": "\uf1fc", "filter": "\uf0b0", "sync": "\uf021", "active": "\uf0ae", "bell": "\uf0f3", "help": "\uf059",
		"bar": "▍", "sel": "▌", "rule": "╌", "chip_l": "\ue0b6", "chip_r": "\ue0b4",
	},
	"unicode": {
		"app": "◈", "claude": "✳", "codex": "⬢",
		"waiting": "●", "idle": "●", "done": "○", "error": "✗", "working": "●",
		"goal": "→", "window": "▣", "tmux": "▤", "nowindow": "∅", "sub": "⧉", "model": "", "clock": "",
		"impl": "◆", "plan": "◇", "fix": "✚", "review": "◎", "ops": "⚙", "doc": "▤",
		"project": "", "home": "", "go": "", "node": "", "python": "", "rust": "", "astro": "", "git": "",
		"theme": "", "filter": "/", "sync": "⟳", "active": "", "bell": "", "help": "?",
		"bar": "▍", "sel": "▌", "rule": "╌", "chip_l": "", "chip_r": "",
	},
	"ascii": {
		"app": "#", "claude": "C", "codex": "X",
		"waiting": "!", "idle": "-", "done": "v", "error": "x", "working": "*",
		"goal": "->", "window": "[w]", "tmux": "[t]", "nowindow": "[-]", "sub": "sub", "model": "", "clock": "",
		"impl": "", "plan": "", "fix": "", "review": "", "ops": "", "doc": "",
		"project": "", "home": "", "go": "", "node": "", "python": "", "rust": "", "astro": "", "git": "",
		"theme": "", "filter": "/", "sync": "~", "active": "", "bell": "", "help": "?",
		"bar": "|", "sel": ">", "rule": "-", "chip_l": "", "chip_r": "",
	},
}

var spinners = map[string][]string{
	"braille": {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	"dots":    {"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
	"line":    {"|", "/", "-", "\\"},
	"none":    {"●"},
}

var icons map[string]string

func loadIcons(cfg IconConfig) {
	set, ok := iconSets[cfg.Set]
	if !ok {
		set = iconSets["nerd"]
	}
	icons = map[string]string{}
	for k, v := range set {
		icons[k] = v
	}
	for k, v := range cfg.Override {
		icons[k] = v
	}
}

func ic(name string) string { return icons[name] }

// icf: icon followed by a space if the icon is not empty.
func icf(name string) string {
	if v := icons[name]; v != "" {
		return v + " "
	}
	return ""
}

// ── text tables (es/en) ─────────────────────────────────────────────────────
var lang = "en"

var texts = map[string]map[string]string{
	"es": {
		"waiting": "espera", "working": "trabaja", "idle": "idle", "done": "hecha", "error": "error",
		"n_active": "activas", "n_working": "trabajan", "n_waiting": "esperan",
		"k_go": "ir a la ventana", "k_attach": "attach", "k_new": "nueva", "k_rename": "renombrar", "k_type": "tipo",
		"k_archive": "archivar", "k_done": "hechas", "k_filter": "filtrar", "k_quit": "salir", "k_help": "ayuda",
		"goal": "objetivo", "last": "último", "window": "ventana", "to_go": "↵ para ir", "not_found": "no localizada",
		"ended": "sesión terminada", "attach_hint": "a para attach/resume", "no_goal": "sin objetivo aún: se rellena en el próximo prompt",
		"no_goal_short": "sin objetivo aún", "no_window": "sin ventana", "turns": "turnos", "since": "desde", "type_by": "tipo por", "title_by": "título por",
		"empty": "sin sesiones. Lanza claude o codex y aparecerán aquí; n crea una nueva.",
		"now":   "ahora", "yesterday": "ayer", "archived": "archivada", "syncing": "sincronizando…",
		"need_tmux": "n necesita tmux: abre tmux y vuelve a intentarlo", "launched": "lanzada", "renamed": "renombrada",
		"no_term": "%s no tiene terminal abierta: usa a para attach/resume", "no_win": "no encuentro la ventana de %s",
		"filter_hint": "esc limpia · ↵ fija", "new_hint": "proyecto/tipo: descripción → ", "new_tail": "↵ lanza en tmux · esc cancela",
		"rename_hint": "nuevo título → ", "rename_tail": "↵ aplica · esc cancela",
		"help_title": "teclas", "help_close": "cualquier tecla cierra",
		"h_nav": "↑/k ↓/j g G  moverse", "h_go": "↵ l →  ir a la ventana de la sesión (Hyprland/Sway/tmux)",
		"h_attach": "a  attach (sesión en background) o resume (terminada)", "h_new": "n  nueva sesión Claude en una ventana tmux",
		"h_rename": "r  renombrar (proyecto/tipo: descripción)", "h_type": "t  cambiar tipo (impl→plan→fix→review→ops→doc)",
		"h_archive": "x  archivar la ficha", "h_done": "d  mostrar/ocultar terminadas", "h_sync": "s  sincronizar ahora",
		"h_filter": "/  filtrar por texto", "h_quit": "q  salir",
	},
	"en": {
		"waiting": "waiting", "working": "working", "idle": "idle", "done": "done", "error": "error",
		"n_active": "active", "n_working": "working", "n_waiting": "waiting",
		"k_go": "go to window", "k_attach": "attach", "k_new": "new", "k_rename": "rename", "k_type": "type",
		"k_archive": "archive", "k_done": "done", "k_filter": "filter", "k_quit": "quit", "k_help": "help",
		"goal": "goal", "last": "last", "window": "window", "to_go": "↵ to go", "not_found": "not found",
		"ended": "session ended", "attach_hint": "a to attach/resume", "no_goal": "no goal yet: filled in on the next prompt",
		"no_goal_short": "no goal yet", "no_window": "no window", "turns": "turns", "since": "since", "type_by": "type by", "title_by": "title by",
		"empty": "no sessions. Start claude or codex and they show up here; n starts a new one.",
		"now":   "now", "yesterday": "yesterday", "archived": "archived", "syncing": "syncing…",
		"need_tmux": "n needs tmux: open tmux and try again", "launched": "launched", "renamed": "renamed",
		"no_term": "%s has no open terminal: use a to attach/resume", "no_win": "cannot find the window of %s",
		"filter_hint": "esc clears · ↵ keeps", "new_hint": "project/type: description → ", "new_tail": "↵ launches in tmux · esc cancels",
		"rename_hint": "new title → ", "rename_tail": "↵ applies · esc cancels",
		"help_title": "keys", "help_close": "any key closes",
		"h_nav": "↑/k ↓/j g G  move", "h_go": "↵ l →  go to the session's window (Hyprland/Sway/tmux)",
		"h_attach": "a  attach (background session) or resume (finished)", "h_new": "n  new Claude session in a tmux window",
		"h_rename": "r  rename (project/type: description)", "h_type": "t  cycle type (impl→plan→fix→review→ops→doc)",
		"h_archive": "x  archive the card", "h_done": "d  show/hide finished", "h_sync": "s  sync now",
		"h_filter": "/  filter by text", "h_quit": "q  quit",
	},
}

func T(key string) string {
	if v, ok := texts[lang][key]; ok {
		return v
	}
	if v, ok := texts["en"][key]; ok {
		return v
	}
	return key
}
