//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// macOS has no compositor to query. What it does have is scriptable terminals:
// iTerm2 and Terminal.app expose the tty of every tab, and a tty is exactly what
// identifies the window a session lives in. airboss-mac-window does the
// AppleScript; this file only speaks to it.
//
//	airboss-mac-window list   → app \t winid \t tab \t tty \t apppid \t title
//
// Terminals that do not script their tabs (Ghostty, kitty, WezTerm, Alacritty,
// Warp, …) are listed with tty "-": they can be raised as an app but no tab can
// be singled out inside them.

func macWindowHelper() string { return helperPath("airboss-mac-window") }

func listWindows() []window {
	bin := macWindowHelper()
	if bin == "" {
		return nil
	}
	out, err := exec.Command(bin, "list").Output()
	if err != nil {
		return nil
	}
	var ws []window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(f[4])
		w := window{Address: f[1], Pid: pid, Class: f[0], Title: f[5], backend: "macos"}
		if f[3] != "-" {
			w.tty = f[3]
		}
		ws = append(ws, w)
	}
	return ws
}

// ppidOf goes through ps: there is no /proc on macOS.
func ppidOf(pid int) int {
	out, err := runOut("ps", "-o", "ppid=", "-p", strconv.Itoa(pid))
	if err != nil {
		return 0
	}
	pp, _ := strconv.Atoi(strings.TrimSpace(out))
	return pp
}

// sessionTty is the tty of the terminal tab showing the session: the tmux
// client's when it runs in a pane, its own otherwise.
func sessionTty(s *Session) string {
	if s.TmuxTarget != "" {
		if t := tmuxClientTty(s.TmuxTarget); t != "" {
			return t
		}
	}
	if s.Pid != nil && *s.Pid > 0 {
		return ttyOfPid(*s.Pid)
	}
	return ""
}

// resolveWindow matches by tty first, since that is the only mapping that picks
// the right tab. Failing that it walks the pid ancestry up to the terminal
// application and returns an app-level window: raising the app is honest, while
// guessing a tab inside it would jump somewhere the session is not.
func resolveWindow(s *Session, wins []window) *window {
	if len(wins) == 0 {
		return nil
	}
	if tty := sessionTty(s); tty != "" {
		for i := range wins {
			if wins[i].tty == tty {
				return &wins[i]
			}
		}
	}
	pids := []int{}
	if s.Pid != nil && *s.Pid > 0 {
		pids = append(pids, *s.Pid)
	}
	if s.TmuxTarget != "" {
		if p := tmuxClientPid(s.TmuxTarget); p > 0 {
			pids = append(pids, p)
		}
	}
	for _, p := range pids {
		if w := windowOfPid(p, wins); w != nil {
			app := *w // app-level copy: no tab, so focus only raises the app
			app.tty = ""
			app.Address = "-"
			app.Title = w.Class
			return &app
		}
	}
	return nil
}

func focusWindow(w *window) error {
	bin := macWindowHelper()
	if bin == "" {
		return fmt.Errorf("airboss-mac-window not found")
	}
	if w.tty != "" {
		if err := exec.Command(bin, "focus", w.Class, w.tty).Run(); err != nil {
			return fmt.Errorf("osascript: %v", err)
		}
		return nil
	}
	if err := exec.Command(bin, "focus-app", w.Class).Run(); err != nil {
		return fmt.Errorf("osascript: %v", err)
	}
	return nil
}
