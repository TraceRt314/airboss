// airboss — a TUI to see what each Claude Code and Codex CLI session is
// doing, know which one is waiting on you, and jump to its terminal with one key.
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
	cfgPath := flag.String("config", "", "path to config.toml (default ~/.config/airboss/config.toml)")
	theme := flag.String("theme", "", "builtin theme: catppuccin-mocha, tokyo-night, gruvbox-dark, nord, dracula, rose-pine, everforest, kanagawa, catppuccin-latte")
	iconSet := flag.String("icons", "", "icon set: nerd | unicode | ascii")
	lng := flag.String("lang", "", "language: es | en")
	showVersion := flag.Bool("version", false, "show version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "airboss-tui %s\n\nusage: airboss-tui [flags] [sync | color <key> | themes | windows]\n\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("airboss-tui", version)
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
	case "color": // airboss-tui color accent → #89b4fa (for tmux, starship, scripts)
		fmt.Println(pal[flag.Arg(1)])
		return
	case "windows": // diagnostic: what airboss sees and where it would jump
		printWindows(&cfg)
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
