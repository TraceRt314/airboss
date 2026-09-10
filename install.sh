#!/usr/bin/env bash
# airboss installer: builds the TUI, links the scripts into ~/.local/bin, installs
# the periodic sync (systemd user timer on Linux, launchd agent on macOS) and
# (optionally) registers the hooks in Claude Code and Codex CLI. Idempotent;
# re-run after `git pull`.
#
#   ./install.sh            build + link + timer
#   ./install.sh --hooks    also merge hooks into ~/.claude/settings.json and ~/.codex
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$HOME/.local/bin"
OS="$(uname -s)"
say()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*"; }

for dep in jq go; do
  command -v "$dep" >/dev/null || { warn "missing $dep"; exit 1; }
done
if [ "$OS" = Darwin ]; then
  command -v tmux >/dev/null || warn "optional: tmux not found"
  command -v terminal-notifier >/dev/null || warn "optional: terminal-notifier not found (falling back to osascript notifications)"
else
  for dep in tmux notify-send hyprctl swaymsg; do
    command -v "$dep" >/dev/null || warn "optional: $dep not found"
  done
fi

say "building airboss-tui"
mkdir -p "$BIN"
( cd "$HERE" && go build -ldflags "-X main.version=$(git -C "$HERE" describe --tags --always --dirty 2>/dev/null || echo dev)" -o "$BIN/airboss-tui" . )

say "linking scripts into $BIN"
SCRIPTS="lib-airboss.sh agent-event agent-board-sync agent-title airboss airboss-focus airboss-notify airboss-tmux-segment"
[ "$OS" = Darwin ] && SCRIPTS="$SCRIPTS airboss-mac-window"
for s in $SCRIPTS; do
  chmod +x "$HERE/scripts/$s"
  ln -sfn "$HERE/scripts/$s" "$BIN/$s"
done

mkdir -p "$HOME/.local/state/agent-board/sessions" "$HOME/.config/airboss"
[ -f "$HOME/.config/airboss/config.toml" ] || cp "$HERE/config.example.toml" "$HOME/.config/airboss/config.toml"

# ── periodic reconcile every 5 s ────────────────────────────────────────────
if [ "$OS" = Darwin ]; then
  say "installing launchd agent (agent-board-sync every 5 s)"
  PLIST="$HOME/Library/LaunchAgents/com.airboss.sync.plist"
  mkdir -p "$HOME/Library/LaunchAgents"
  # launchd agents start with a minimal PATH: hand them the one that has jq,
  # tmux, git and the claude CLI, or every run silently does nothing.
  LPATH="$BIN:$(dirname "$(command -v jq)"):/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
  CLAUDE_DIR="$(command -v claude >/dev/null && dirname "$(command -v claude)" || true)"
  [ -n "${CLAUDE_DIR:-}" ] && LPATH="$CLAUDE_DIR:$LPATH"
  sed -e "s|__HOME__|$HOME|g" -e "s|__PATH__|$LPATH|g" "$HERE/launchd/com.airboss.sync.plist" > "$PLIST"
  DOMAIN="gui/$(id -u)"
  # bootout is asynchronous: bootstrapping again before the old job is gone
  # fails with "Input/output error", so wait for it to disappear first.
  launchctl bootout "$DOMAIN/com.airboss.sync" >/dev/null 2>&1 || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    launchctl print "$DOMAIN/com.airboss.sync" >/dev/null 2>&1 || break
    sleep 0.5
  done
  if launchctl bootstrap "$DOMAIN" "$PLIST" 2>/dev/null; then
    launchctl kickstart "$DOMAIN/com.airboss.sync" >/dev/null 2>&1 || true
  else
    warn "launchctl bootstrap failed; the previous agent is still loaded. Run:"
    warn "  launchctl bootout $DOMAIN/com.airboss.sync && launchctl bootstrap $DOMAIN $PLIST"
  fi
elif command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  say "installing systemd user timer (agent-board-sync every 5 s)"
  mkdir -p "$HOME/.config/systemd/user"
  rm -f "$HOME/.config/systemd/user/agent-board-sync.service" "$HOME/.config/systemd/user/agent-board-sync.timer"
  cp "$HERE/systemd/agent-board-sync.service" "$HERE/systemd/agent-board-sync.timer" "$HOME/.config/systemd/user/"
  systemctl --user daemon-reload
  systemctl --user enable --now agent-board-sync.timer >/dev/null
else
  warn "no systemd: run 'agent-board-sync' from cron every few seconds, or rely on the TUI's own sync"
fi

# ── hooks ───────────────────────────────────────────────────────────────────
if [ "${1:-}" = "--hooks" ]; then
  S="$HOME/.claude/settings.json"
  if [ -f "$S" ]; then
    cp "$S" "$S.bak.$(date +%s)"
    say "merging hooks into $S (backup kept)"
    jq --slurpfile h "$HERE/hooks/claude-hooks.json" '
      .hooks = ((.hooks // {}) as $cur
        | reduce ($h[0] | to_entries[]) as $e ($cur;
            .[$e.key] = (((.[$e.key] // []) | map(select(any(.hooks[]?; .command | test("agent-event")) | not))) + $e.value)))' \
      "$S" > "$S.tmp"
    # written through, not moved into place: $S may be a symlink into a dotfiles repo
    cat "$S.tmp" > "$S" && rm -f "$S.tmp"
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

if [ "$OS" = Darwin ]; then
  say "done. Run: airboss   (bind it in iTerm2: Prefs → Keys → Key Binding → Send Text: \"airboss\\n\")"
else
  say "done. Run: airboss   (or bind it: Hyprland  o.bind(\"SUPER + F1\", \"airboss\", { tui = \"airboss\", focus = true }))"
fi
