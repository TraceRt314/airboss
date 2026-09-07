// torre — Torre de Control: una TUI para ver qué hace cada sesión de Claude Code
// y Codex CLI, saber cuál te espera y saltar a su terminal con una tecla.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "", "ruta de config.toml (por defecto ~/.config/torre/config.toml)")
	theme := flag.String("theme", "", "tema builtin: catppuccin-mocha, tokyo-night, gruvbox-dark, nord, dracula, rose-pine, everforest, kanagawa, catppuccin-latte")
	iconSet := flag.String("icons", "", "juego de iconos: nerd | unicode | ascii")
	lng := flag.String("lang", "", "idioma: es | en")
	showVersion := flag.Bool("version", false, "versión")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "torre-tui %s\n\nuso: torre-tui [flags] [sync | color <clave> | themes]\n\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("torre-tui", version)
		return
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	if *theme != "" {
		cfg.Theme.Source, cfg.Theme.Name = "builtin", *theme
	}
	if *iconSet != "" {
		cfg.Icons.Set = *iconSet
	}
	if *lng != "" {
		cfg.UI.Lang = *lng
	}
	lang = cfg.UI.Lang
	loadIcons(cfg.Icons)
	loadTheme(cfg.Theme)

	switch flag.Arg(0) {
	case "sync":
		exec.Command(syncBin()).Run()
		return
	case "color": // torre-tui color accent → #89b4fa (para tmux, starship, scripts)
		fmt.Println(pal[flag.Arg(1)])
		return
	case "themes":
		for n := range builtinThemes {
			fmt.Println(n)
		}
		return
	}
	p := tea.NewProgram(newModel(&cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
