<h1 align="center">󱚣 torre</h1>

<p align="center"><b>A control tower for your AI coding agents.</b><br>
One TUI that shows every Claude Code and Codex CLI session on your machine, grouped by project and task type, tells you which one is waiting for you, and jumps to its terminal window with <kbd>Enter</kbd>.</p>

<p align="center"><img src="docs/screenshot.svg" alt="torre screenshot" width="100%"></p>

<p align="center">
<code>Go · Bubble Tea · ~1.5k lines</code> · <code>no daemon, no account, no Electron</code> · <code>Hyprland · Sway · tmux</code> · <code>follows your Omarchy theme</code>
</p>

---

## Why

You open a terminal per task, hand each one to an agent, and then you are alt-tabbing through eight windows to find out which agent finished, which one is stuck on a permission prompt, and which one has been idle for an hour. `torre` answers that from a single window:

- **State per session**, fed by the agents' own hooks: `working`, `waiting` (permission or question), `idle`, `error`, `done`. Codex sessions started before hooks existed are picked up from their rollout files.
- **Grouped by project and typed by task** (`impl`, `plan`, `fix`, `review`, `ops`, `doc`). Titles follow the convention `project/type: description`; a background classifier names untitled sessions for you.
- **Enter goes to the terminal.** torre finds the compositor window that owns the agent process (walking `/proc` ancestors, tmux clients or window titles) and focuses it on Hyprland or Sway, then selects the tmux window and pane.
- **Desktop notifications** when an agent waits for you, errors or finishes a turn, for both CLIs, through one script.
- **Looks like your desktop.** Palette from the active Omarchy theme, nine built-in themes, three icon sets, every glyph and color overridable in a small TOML.

## Install

Requirements: Linux, Go ≥ 1.22, `jq`. Optional: `tmux`, `notify-send`, a [Nerd Font](https://www.nerdfonts.com/), Hyprland or Sway for window focus.

```bash
git clone https://github.com/TraceRt314/torre.git ~/torre && cd ~/torre
./install.sh --hooks      # builds torre-tui, links scripts into ~/.local/bin,
                          # installs the systemd user timer, registers the hooks
torre
```

`--hooks` merges `hooks/claude-hooks.json` into `~/.claude/settings.json` (backup kept) and writes `~/.codex/hooks.json` plus a `notify` line in `~/.codex/config.toml`. Codex asks you once to trust the hooks with `/hooks`. Without `--hooks` nothing outside `~/.local/bin`, `~/.config/torre` and the systemd unit is touched.

Bind it to a key. Omarchy / Hyprland (`~/.config/hypr/bindings.lua`):

```lua
o.bind("SUPER + F1", "torre", { tui = "torre", focus = true })
```

Sway: `bindsym $mod+F1 exec foot -a torre torre`.

## Keys

| key | action |
|---|---|
| <kbd>↵</kbd> <kbd>l</kbd> <kbd>→</kbd> | go to the session's window (compositor + tmux) |
| <kbd>a</kbd> | attach a background session or resume a finished one |
| <kbd>n</kbd> | new Claude session in a tmux window (`project/type: description`) |
| <kbd>r</kbd> | rename (the new title is pushed to Claude's session list and `/resume`) |
| <kbd>t</kbd> | cycle the task type |
| <kbd>x</kbd> | archive the card |
| <kbd>d</kbd> | show / hide finished sessions |
| <kbd>/</kbd> | filter by text |
| <kbd>s</kbd> | sync now |
| <kbd>?</kbd> | help |

## How it works

```
Claude Code hooks ─┐                       ┌─ torre-tui        (this TUI)
Codex CLI hooks ───┼─▶ agent-event ─▶ state/ ┼─ statusline / tmux segment
codex notify ──────┘         ▲              └─ notify-send
                             │
systemd timer, 5 s ─▶ agent-board-sync   (claude agents --json, /proc, Codex rollouts)
```

- `scripts/agent-event` receives every hook event from both CLIs on stdin, normalizes it into one JSON card per session under `~/.local/state/agent-board/sessions/`, sets the canonical title and sends desktop notifications on the transitions that matter.
- `scripts/agent-board-sync` reconciles that state with reality every five seconds: `claude agents --json` is authoritative for Claude; for Codex it finds live processes, reads the open rollout file to know whether a turn is in progress (`task_started` without `task_complete`), and marks dead sessions `done`. It runs under a lock and reads rollouts incrementally, so a 500 MB rollout costs nothing.
- `scripts/agent-title` asks a small model (Haiku, no hooks, no transcript) for a `type: description` for sessions you did not name. Your own `/rename` always wins.
- `torre-tui` reads the cards, resolves each session's window and draws. It never writes to the agents; the only state it owns is the cards.

Everything is plain files and shell, so `cat ~/.local/state/agent-board/sessions/*.json` is a valid dashboard too. The `summary.json` next to it feeds a [tmux segment](scripts/torre-tmux-segment) and can feed starship or waybar.

## Customize

`~/.config/torre/config.toml` (created from [`config.example.toml`](config.example.toml) on install). All keys are optional.

```toml
[theme]
source = "auto"             # auto | omarchy | builtin
name = "tokyo-night"        # catppuccin-mocha, catppuccin-latte, tokyo-night, gruvbox-dark,
                            # nord, dracula, rose-pine, everforest, kanagawa
[theme.colors]
accent = "#ff79c6"          # any palette key: background, foreground, muted, red, green…

[icons]
set = "nerd"                # nerd | unicode | ascii
[icons.override]
claude = "\U000F06A9"       # Nerd Font glyphs as \u / \U escapes
waiting = ""

[ui]
title = "CONTROL TOWER"
lang = "en"                 # es | en
show_goal = true
details = true
border = "rounded"          # rounded | square | none
spinner = "dots"            # braille | dots | line | none

[types.fix]
icon = ""
color = "orange"
label = "bug"

[projects.prob-monorepo]
icon = ""
color = "blue"
label = "Problyx"
```

Flags override the file for a one-off: `torre-tui -theme nord -icons unicode -lang en`. `torre-tui color accent` prints a palette color for scripts, `torre-tui themes` lists the built-ins, and [`themes/`](themes/) has each palette as TOML in the same format as Omarchy's `colors.toml`, so you can drop your own next to them.

**Icon keys:** `app claude codex waiting working idle done error goal window tmux nowindow sub model clock impl plan fix review ops doc project home go node python rust astro git theme filter sync active bell help bar sel rule chip_l chip_r`. Project icons are auto-detected from `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `astro.config.*` or `.git` unless you set one.

## Session naming

torre expects `project/type: description`, e.g. `epsilon/impl: dynamic reviews`. Set it with `claude --name "…"`, `/rename` in either CLI, or <kbd>r</kbd> here. A free-text `/rename` is normalized to the format; untitled sessions get a provisional title from the first prompt and a proper one from the classifier a few seconds later. The type drives the color and, if you use the optional `cl` launcher from the author's dotfiles, the model and effort profile.

## Compared with

COMPARISON_TABLE

torre is the small, terminal-native option: it does not run your agents, isolate worktrees or offer a web UI. It watches the sessions you already have, in the terminals you already use, and gets out of the way.

## Status

Built on Arch + Omarchy with Claude Code 2.1 and Codex CLI 0.150. Hyprland focus uses the Lua dispatcher of Hyprland ≥ 0.56 with the classic syntax as fallback. macOS is not supported (no `/proc`, no compositor backend); a Yabai/AeroSpace backend would be a small addition in `window.go`. PRs welcome.

## License

MIT.
