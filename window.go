package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// window is a terminal window hosting a session: a compositor client on
// Linux (Hyprland, Sway), a terminal-emulator window or tab on macOS.
type window struct {
	Address   string `json:"address"`
	Pid       int    `json:"pid"`
	Class     string `json:"class"`
	Title     string `json:"title"`
	Workspace struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"workspace"`
	backend string // hyprland | sway | macos
	tty     string // macOS: the tab's tty, the only reliable pid→tab mapping
}

func runOut(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

func hasCmd(name string) bool { _, err := exec.LookPath(name); return err == nil }

// helperPath finds one of the airboss shell helpers: installed next to the
// binary in ~/.local/bin, alongside a binary built in the repo, or on PATH.
func helperPath(name string) string {
	var cands []string
	if home := os.Getenv("HOME"); home != "" {
		cands = append(cands, filepath.Join(home, ".local/bin", name))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands, filepath.Join(dir, name), filepath.Join(dir, "scripts", name))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return c
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func windowOfPid(pid int, wins []window) *window {
	byPid := map[int]*window{}
	for i := range wins {
		byPid[wins[i].Pid] = &wins[i]
	}
	for p, n := pid, 0; p > 1 && n < 12; p, n = ppidOf(p), n+1 {
		if w, ok := byPid[p]; ok {
			return w
		}
	}
	return nil
}

// tmuxClientPid: pid of the tmux client displaying the target's session (#S:#I.#P).
func tmuxClientPid(target string) int {
	sess, _, _ := strings.Cut(target, ":")
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_pid} #{session_name}").Output()
	if err != nil {
		return 0
	}
	first := 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		if first == 0 {
			first = pid
		}
		if f[1] == sess {
			return pid
		}
	}
	return first
}

// tmuxClientTty: tty of the tmux client showing the target's session. Inside
// tmux the agent's own tty is the pane's pty, which owns no terminal window;
// the client's tty is the one the emulator knows about.
func tmuxClientTty(target string) string {
	sess, _, _ := strings.Cut(target, ":")
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_tty} #{session_name}").Output()
	if err != nil {
		return ""
	}
	first := ""
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		if first == "" {
			first = f[0]
		}
		if f[1] == sess {
			return f[0]
		}
	}
	return first
}

// ttyOfPid returns /dev/ttysNNN for a process attached to a terminal.
func ttyOfPid(pid int) string {
	out, err := runOut("ps", "-o", "tty=", "-p", strconv.Itoa(pid))
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(out)
	switch t {
	case "", "?", "??", "-":
		return ""
	}
	if !strings.HasPrefix(t, "/dev/") {
		t = "/dev/" + t
	}
	return t
}

// focusTmux switches the client, window and pane for a #S:#I.#P target.
func focusTmux(target string) {
	if target == "" || !hasCmd("tmux") {
		return
	}
	sess, rest, _ := strings.Cut(target, ":")
	win, pane, _ := strings.Cut(rest, ".")
	if os.Getenv("TMUX") != "" {
		exec.Command("tmux", "switch-client", "-t", sess).Run()
	} else {
		out, _ := runOut("tmux", "list-clients", "-F", "#{client_name} #{session_name}")
		client := ""
		for _, l := range strings.Split(out, "\n") {
			f := strings.Fields(l)
			if len(f) == 2 && client == "" {
				client = f[0]
			}
			if len(f) == 2 && f[1] == sess {
				client = f[0]
				break
			}
		}
		if client != "" {
			exec.Command("tmux", "switch-client", "-c", client, "-t", sess).Run()
		}
	}
	exec.Command("tmux", "select-window", "-t", sess+":"+win).Run()
	if pane != "" {
		exec.Command("tmux", "select-pane", "-t", sess+":"+win+"."+pane).Run()
	}
}

func winLabel(w *window) string {
	if w == nil {
		return ""
	}
	parts := []string{}
	if ws := w.Workspace.Name; ws != "" {
		parts = append(parts, "ws "+ws)
	}
	if w.Class != "" {
		parts = append(parts, w.Class)
	}
	if w.Title != "" {
		parts = append(parts, trunc(w.Title, 40))
	}
	return strings.Join(parts, " · ")
}

// printWindows dumps the terminal windows airboss can see and the one it would
// jump to for each session: the fastest way to tell whether window resolution
// works on a given desktop.
func printWindows(cfg *Config) {
	wins := listWindows()
	fmt.Printf("windows (%d)\n", len(wins))
	for i := range wins {
		w := &wins[i]
		tty := w.tty
		if tty == "" {
			tty = "-"
		}
		fmt.Printf("  %-10s addr=%-14s pid=%-7d tty=%-14s %s\n", w.backend, w.Address, w.Pid, tty, trunc(w.Title, 50))
	}
	ss := loadSessions(stateDir(cfg))
	fmt.Printf("\nsessions (%d)\n", len(ss))
	for i := range ss {
		s := &ss[i]
		pid := 0
		if s.Pid != nil {
			pid = *s.Pid
		}
		target := resolveWindow(s, wins)
		where := "— no window"
		if target != nil {
			where = "→ " + winLabel(target)
		}
		fmt.Printf("  %-8s %-44s pid=%-7d tmux=%-12s %s\n", s.State, trunc(shortName(s), 42), pid, orDash(s.TmuxTarget), where)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
