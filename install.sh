#!/usr/bin/env bash
# torre installer: builds the TUI, links the scripts into ~/.local/bin, installs
# the systemd user timer and (optionally) registers the hooks in Claude Code and
# Codex CLI. Idempotent; re-run after `git pull`.
#
#   ./install.sh            build + link + timer
#   ./install.sh --hooks    also merge hooks into ~/.claude/settings.json and ~/.codex
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$HOME/.local/bin"
say()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*"; }

for dep in jq go; do
  command -v "$dep" >/dev/null || { warn "missing $dep"; exit 1; }
done
for dep in tmux notify-send hyprctl swaymsg; do
  command -v "$dep" >/dev/null || warn "optional: $dep not found"
done

say "building torre-tui"
mkdir -p "$BIN"
( cd "$HERE" && go build -ldflags "-X main.version=$(git -C "$HERE" describe --tags --always --dirty 2>/dev/null || echo dev)" -o "$BIN/torre-tui" . )

say "linking scripts into $BIN"
for s in agent-event agent-board-sync agent-title torre torre-focus torre-tmux-segment; do
  ln -sfn "$HERE/scripts/$s" "$BIN/$s"
done

mkdir -p "$HOME/.local/state/agent-board/sessions" "$HOME/.config/torre"
[ -f "$HOME/.config/torre/config.toml" ] || cp "$HERE/config.example.toml" "$HOME/.config/torre/config.toml"

if command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  say "installing systemd user timer (agent-board-sync every 5 s)"
  mkdir -p "$HOME/.config/systemd/user"
  rm -f "$HOME/.config/systemd/user/agent-board-sync.service" "$HOME/.config/systemd/user/agent-board-sync.timer"
  cp "$HERE/systemd/agent-board-sync.service" "$HERE/systemd/agent-board-sync.timer" "$HOME/.config/systemd/user/"
  systemctl --user daemon-reload
  systemctl --user enable --now agent-board-sync.timer >/dev/null
else
  warn "no systemd: run 'agent-board-sync' from cron every few seconds, or rely on the TUI's own sync"
fi

if [ "${1:-}" = "--hooks" ]; then
  S="$HOME/.claude/settings.json"
  if [ -f "$S" ]; then
    cp "$S" "$S.bak.$(date +%s)"
    say "merging hooks into $S (backup kept)"
    jq --slurpfile h "$HERE/hooks/claude-hooks.json" '
      .hooks = ((.hooks // {}) as $cur
        | reduce ($h[0] | to_entries[]) as $e ($cur;
            .[$e.key] = (((.[$e.key] // []) | map(select(any(.hooks[]?; .command | test("agent-event")) | not))) + $e.value)))' \
      "$S" > "$S.tmp" && mv "$S.tmp" "$S"
  else
    warn "$S not found: copy hooks/claude-hooks.json into it by hand"
  fi
  C="$HOME/.codex"
  if [ -d "$C" ]; then
    [ -f "$C/hooks.json" ] && cp "$C/hooks.json" "$C/hooks.json.bak.$(date +%s)"
    sed "s|\$HOME|$HOME|g" "$HERE/hooks/codex-hooks.json" > "$C/hooks.json"
    say "wrote $C/hooks.json (open codex and accept them with /hooks)"
    if ! grep -q 'agent-event' "$C/config.toml" 2>/dev/null; then
      sed "s|/absolute/path/to/.local/bin|$BIN|" "$HERE/hooks/codex-config.snippet.toml" >> "$C/config.toml"
      say "appended notify + tui notifications to $C/config.toml"
    fi
  fi
fi

say "done. Run: torre   (or bind it: Hyprland  o.bind(\"SUPER + F1\", \"torre\", { tui = \"torre\", focus = true }))"
