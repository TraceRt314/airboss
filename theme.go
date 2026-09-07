package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
)

// Palette with the keys from Omarchy's colors.toml. Any builtin theme or the
// active Omarchy theme fills these keys; [theme.colors] overrides them.
var pal = map[string]string{}
var themeName = ""
var themeDark = true

var builtinThemes = map[string]map[string]string{
	"catppuccin-mocha": {
		"mode": "dark", "accent": "#89b4fa", "background": "#1e1e2e", "lighter_background": "#313244", "selection": "#45475a", "muted": "#585b70",
		"foreground": "#cdd6f4", "dark_foreground": "#6c7086", "light_foreground": "#bac2de",
		"red": "#f38ba8", "yellow": "#f9e2af", "orange": "#fab387", "green": "#a6e3a1", "cyan": "#94e2d5", "blue": "#89b4fa", "magenta": "#f5c2e7",
	},
	"catppuccin-latte": {
		"mode": "light", "accent": "#1e66f5", "background": "#eff1f5", "lighter_background": "#ccd0da", "selection": "#bcc0cc", "muted": "#9ca0b0",
		"foreground": "#4c4f69", "dark_foreground": "#8c8fa1", "light_foreground": "#6c6f85",
		"red": "#d20f39", "yellow": "#df8e1d", "orange": "#fe640b", "green": "#40a02b", "cyan": "#179299", "blue": "#1e66f5", "magenta": "#ea76cb",
	},
	"tokyo-night": {
		"mode": "dark", "accent": "#7aa2f7", "background": "#1a1b26", "lighter_background": "#292e42", "selection": "#33467c", "muted": "#3b4261",
		"foreground": "#c0caf5", "dark_foreground": "#565f89", "light_foreground": "#a9b1d6",
		"red": "#f7768e", "yellow": "#e0af68", "orange": "#ff9e64", "green": "#9ece6a", "cyan": "#7dcfff", "blue": "#7aa2f7", "magenta": "#bb9af7",
	},
	"gruvbox-dark": {
		"mode": "dark", "accent": "#fabd2f", "background": "#282828", "lighter_background": "#3c3836", "selection": "#504945", "muted": "#665c54",
		"foreground": "#ebdbb2", "dark_foreground": "#928374", "light_foreground": "#d5c4a1",
		"red": "#fb4934", "yellow": "#fabd2f", "orange": "#fe8019", "green": "#b8bb26", "cyan": "#8ec07c", "blue": "#83a598", "magenta": "#d3869b",
	},
	"nord": {
		"mode": "dark", "accent": "#88c0d0", "background": "#2e3440", "lighter_background": "#3b4252", "selection": "#434c5e", "muted": "#4c566a",
		"foreground": "#eceff4", "dark_foreground": "#7b88a1", "light_foreground": "#d8dee9",
		"red": "#bf616a", "yellow": "#ebcb8b", "orange": "#d08770", "green": "#a3be8c", "cyan": "#8fbcbb", "blue": "#81a1c1", "magenta": "#b48ead",
	},
	"dracula": {
		"mode": "dark", "accent": "#bd93f9", "background": "#282a36", "lighter_background": "#44475a", "selection": "#44475a", "muted": "#6272a4",
		"foreground": "#f8f8f2", "dark_foreground": "#6272a4", "light_foreground": "#e6e6e6",
		"red": "#ff5555", "yellow": "#f1fa8c", "orange": "#ffb86c", "green": "#50fa7b", "cyan": "#8be9fd", "blue": "#8be9fd", "magenta": "#ff79c6",
	},
	"rose-pine": {
		"mode": "dark", "accent": "#c4a7e7", "background": "#191724", "lighter_background": "#26233a", "selection": "#403d52", "muted": "#6e6a86",
		"foreground": "#e0def4", "dark_foreground": "#908caa", "light_foreground": "#e0def4",
		"red": "#eb6f92", "yellow": "#f6c177", "orange": "#ea9a97", "green": "#9ccfd8", "cyan": "#9ccfd8", "blue": "#31748f", "magenta": "#c4a7e7",
	},
	"everforest": {
		"mode": "dark", "accent": "#a7c080", "background": "#2d353b", "lighter_background": "#3d484d", "selection": "#475258", "muted": "#7a8478",
		"foreground": "#d3c6aa", "dark_foreground": "#859289", "light_foreground": "#d3c6aa",
		"red": "#e67e80", "yellow": "#dbbc7f", "orange": "#e69875", "green": "#a7c080", "cyan": "#83c092", "blue": "#7fbbb3", "magenta": "#d699b6",
	},
	"kanagawa": {
		"mode": "dark", "accent": "#7e9cd8", "background": "#1f1f28", "lighter_background": "#2a2a37", "selection": "#363646", "muted": "#54546d",
		"foreground": "#dcd7ba", "dark_foreground": "#727169", "light_foreground": "#c8c093",
		"red": "#e82424", "yellow": "#e6c384", "orange": "#ffa066", "green": "#98bb6c", "cyan": "#7aa89f", "blue": "#7e9cd8", "magenta": "#957fb8",
	},
}

func omarchyThemeDir() string {
	if d := os.Getenv("OMARCHY_THEME_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".local/state/omarchy/current/theme")
}

// loadOmarchy reads colors.toml from the active Omarchy theme; returns false if none.
func loadOmarchy() bool {
	b, err := os.ReadFile(filepath.Join(omarchyThemeDir(), "colors.toml"))
	if err != nil {
		return false
	}
	var raw map[string]any
	if _, err := toml.Decode(string(b), &raw); err != nil {
		return false
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			pal[k] = s
		}
	}
	if n, err := os.ReadFile(filepath.Join(filepath.Dir(omarchyThemeDir()), "theme.name")); err == nil {
		themeName = strings.TrimSpace(string(n))
	} else {
		themeName = "omarchy"
	}
	return true
}

func loadTheme(cfg ThemeConfig) {
	base := builtinThemes["catppuccin-mocha"]
	if t, ok := builtinThemes[cfg.Name]; ok {
		base = t
	}
	for k, v := range base {
		pal[k] = v
	}
	themeName = cfg.Name
	switch cfg.Source {
	case "omarchy":
		loadOmarchy()
	case "builtin":
	default: // auto
		loadOmarchy()
	}
	for k, v := range cfg.Colors {
		pal[k] = v
	}
	// alias: if the theme is missing a key, derive it from the ones present
	if pal["accent"] == "" {
		pal["accent"] = pal["blue"]
	}
	if pal["orange"] == "" {
		pal["orange"] = pal["yellow"]
	}
	if pal["selection"] == "" {
		pal["selection"] = pal["lighter_background"]
	}
	themeDark = pal["mode"] != "light"
	// avoids lipgloss probing the terminal (5s on silent ptys)
	lipgloss.SetHasDarkBackground(themeDark)
}

func c(name string) lipgloss.Color {
	if v, ok := pal[name]; ok && v != "" {
		return lipgloss.Color(v)
	}
	if strings.HasPrefix(name, "#") {
		return lipgloss.Color(name)
	}
	return lipgloss.Color(pal["foreground"])
}

func stateColor(s string) lipgloss.Color {
	switch s {
	case "waiting":
		return c("red")
	case "error":
		return c("orange")
	case "working":
		return c("green")
	case "idle":
		return c("yellow")
	default:
		return c("dark_foreground")
	}
}

var defaultTypeColor = map[string]string{"impl": "accent", "plan": "cyan", "fix": "red", "review": "magenta", "ops": "yellow", "doc": "light_foreground"}

func typeColor(cfg *Config, t string) lipgloss.Color {
	if tc, ok := cfg.Types[t]; ok && tc.Color != "" {
		return c(tc.Color)
	}
	if col, ok := defaultTypeColor[t]; ok {
		return c(col)
	}
	return c("light_foreground")
}

func typeIcon(cfg *Config, t string) string {
	if tc, ok := cfg.Types[t]; ok && tc.Icon != "" {
		return tc.Icon
	}
	return ic(t)
}

func typeLabel(cfg *Config, t string) string {
	if tc, ok := cfg.Types[t]; ok && tc.Label != "" {
		return tc.Label
	}
	return t
}
