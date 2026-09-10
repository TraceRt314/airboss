//go:build !darwin

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// listWindows queries the available compositor. With no known compositor it returns
// nil and airboss relies on tmux alone.
func listWindows() []window {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && hasCmd("hyprctl") {
		out, err := exec.Command("hyprctl", "clients", "-j").Output()
		if err == nil {
			var ws []window
			if json.Unmarshal(out, &ws) == nil {
				for i := range ws {
					ws[i].backend = "hyprland"
				}
				return ws
			}
		}
	}
	if os.Getenv("SWAYSOCK") != "" && hasCmd("swaymsg") {
		out, err := exec.Command("swaymsg", "-t", "get_tree").Output()
		if err == nil {
			return swayWindows(out)
		}
	}
	return nil
}

func swayWindows(tree []byte) []window {
	var root map[string]any
	if json.Unmarshal(tree, &root) != nil {
		return nil
	}
	var ws []window
	var walk func(n map[string]any, wsName string)
	walk = func(n map[string]any, wsName string) {
		if t, _ := n["type"].(string); t == "workspace" {
			wsName, _ = n["name"].(string)
		}
		if pid, ok := n["pid"].(float64); ok && pid > 0 {
			w := window{Pid: int(pid), backend: "sway"}
			w.Title, _ = n["name"].(string)
			w.Class, _ = n["app_id"].(string)
			if id, ok := n["id"].(float64); ok {
				w.Address = strconv.Itoa(int(id))
			}
			w.Workspace.Name = wsName
			ws = append(ws, w)
		}
		for _, key := range []string{"nodes", "floating_nodes"} {
			if kids, ok := n[key].([]any); ok {
				for _, k := range kids {
					if km, ok := k.(map[string]any); ok {
						walk(km, wsName)
					}
				}
			}
		}
	}
	walk(root, "")
	return ws
}

// ppidOf reads the parent pid from /proc; comm is wrapped in parentheses and may
// contain spaces, so it's parsed from the last ")".
func ppidOf(pid int) int {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(b)
	i := strings.LastIndex(s, ")")
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return 0
	}
	pp, _ := strconv.Atoi(f[1])
	return pp
}

// resolveWindow: pid ancestors, the pane's tmux client, or window title.
func resolveWindow(s *Session, wins []window) *window {
	if len(wins) == 0 {
		return nil
	}
	if s.Pid != nil && *s.Pid > 0 {
		if w := windowOfPid(*s.Pid, wins); w != nil {
			return w
		}
	}
	if s.TmuxTarget != "" {
		if pid := tmuxClientPid(s.TmuxTarget); pid > 0 {
			if w := windowOfPid(pid, wins); w != nil {
				return w
			}
		}
	}
	for _, needle := range []string{s.Title, s.Name, shortName(s)} {
		if len(needle) < 6 {
			continue
		}
		for i := range wins {
			if strings.Contains(wins[i].Title, needle) {
				return &wins[i]
			}
		}
	}
	return nil
}

// focusWindow focuses the window in the compositor. Hyprland ≥0.56 accepts Lua in
// dispatch; the classic format is kept as a fallback for older versions.
func focusWindow(w *window) error {
	switch w.backend {
	case "hyprland":
		lua := fmt.Sprintf(`hl.dsp.focus({ window = "address:%s" })`, w.Address)
		if err := exec.Command("hyprctl", "dispatch", lua).Run(); err == nil {
			return nil
		}
		if err := exec.Command("hyprctl", "dispatch", "focuswindow", "address:"+w.Address).Run(); err != nil {
			return fmt.Errorf("hyprctl: %v", err)
		}
		return nil
	case "sway":
		if err := exec.Command("swaymsg", fmt.Sprintf("[con_id=%s] focus", w.Address)).Run(); err != nil {
			return fmt.Errorf("swaymsg: %v", err)
		}
		return nil
	}
	return fmt.Errorf("unknown backend")
}
