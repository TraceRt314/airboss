#!/usr/bin/env bash
# lib-airboss.sh — portability layer for the airboss scripts.
#
# Everything here works on GNU/Linux (bash ≥ 4, coreutils) and on macOS
# (bash 3.2, BSD userland, no flock/setsid/notify-send). Sourced, never run.
#
#   . "$(dirname "$0")/lib-airboss.sh"
#
# install.sh links this file next to the scripts in ~/.local/bin, so $0's
# directory resolves it both from the repo and from the installed symlinks.

case "$(uname -s)" in
  Darwin) AB_OS=darwin ;;
  Linux)  AB_OS=linux ;;
  *)      AB_OS=other ;;
esac

# ── locale: C.UTF-8 does not exist on macOS ─────────────────────────────────
ab_setlocale() {
  if [ "$AB_OS" = darwin ]; then
    export LC_ALL=en_US.UTF-8
  else
    export LC_ALL=C.UTF-8
  fi
}

# ── string case (bash 3.2 has no ${v,,} / ${v^}) ────────────────────────────
ab_lc() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }
ab_ucfirst() {
  case "$1" in
    "") ;;
    *) printf '%s%s' "$(printf '%s' "${1%"${1#?}"}" | tr '[:lower:]' '[:upper:]')" "${1#?}" ;;
  esac
}

# ── locks: mkdir is atomic everywhere; flock is Linux-only ──────────────────
# ab_lock <path> [timeout_s]   → 0 on acquire, 1 on timeout
# ab_trylock <path>            → 0 on acquire, 1 if held
# ab_unlock <path>
# A lock older than AB_LOCK_STALE seconds is considered abandoned and stolen,
# so a killed script can never wedge the board.
AB_LOCK_STALE="${AB_LOCK_STALE:-60}"

ab_lock_break_stale() {
  local d="$1.lockd" age owner
  [ -d "$d" ] || return 0
  # the holder recorded its pid: if that process is gone the lock is orphaned
  # (a killed hook, a restarted service) and there is no reason to wait it out
  owner=$(cat "$d/pid" 2>/dev/null)
  if [ -n "$owner" ] && ! kill -0 "$owner" 2>/dev/null; then
    rm -rf "$d"
    return 0
  fi
  age=$(( $(ab_now) - $(ab_stat_mtime "$d") ))
  [ "$age" -gt "$AB_LOCK_STALE" ] && rm -rf "$d"
  return 0
}

ab_trylock() {
  ab_lock_break_stale "$1"
  mkdir "$1.lockd" 2>/dev/null || return 1
  printf '%s' "$$" > "$1.lockd/pid" 2>/dev/null
  return 0
}

# ab_lock_owner <path> → pid holding the lock, empty if free
ab_lock_owner() { cat "$1.lockd/pid" 2>/dev/null; }

ab_lock() {
  local f="$1" timeout="${2:-5}" i=0 steps
  steps=$(( timeout * 10 ))
  while [ "$i" -lt "$steps" ]; do
    ab_trylock "$f" && return 0
    ab_sleep_tenth
    i=$(( i + 1 ))
  done
  return 1
}

ab_unlock() { rm -rf "$1.lockd" 2>/dev/null; return 0; }

# 0.1s sleep: GNU sleep and BSD sleep both accept fractions.
ab_sleep_tenth() { sleep 0.1 2>/dev/null || sleep 1; }

# ── background spawn, detached from the hook's process group ────────────────
# setsid does not exist on macOS; nohup + & + disown is the portable equivalent.
ab_spawn() {
  if [ "$AB_OS" = linux ] && command -v setsid >/dev/null 2>&1; then
    setsid -f "$@" >/dev/null 2>&1 </dev/null
  else
    ( nohup "$@" >/dev/null 2>&1 </dev/null & ) &
  fi
  return 0
}

# ── timeouts: coreutils `timeout` is not part of the macOS base system ──────
# ab_timeout <seconds> <cmd> [args...]
ab_timeout() {
  local secs="$1"; shift
  if command -v timeout >/dev/null 2>&1; then timeout "$secs" "$@"; return $?; fi
  if command -v gtimeout >/dev/null 2>&1; then gtimeout "$secs" "$@"; return $?; fi
  local cmd_pid watch_pid rc
  "$@" &
  cmd_pid=$!
  # watchdog: TERM at the deadline, KILL two seconds later. Its own output goes
  # nowhere so it never holds open the pipe of an enclosing $(...).
  (
    i=0
    while [ "$i" -lt "$secs" ]; do
      kill -0 "$cmd_pid" 2>/dev/null || exit 0
      sleep 1
      i=$(( i + 1 ))
    done
    kill -TERM "$cmd_pid" 2>/dev/null
    sleep 2
    kill -KILL "$cmd_pid" 2>/dev/null
  ) >/dev/null 2>&1 &
  watch_pid=$!
  wait "$cmd_pid" 2>/dev/null
  rc=$?
  kill "$watch_pid" 2>/dev/null
  wait "$watch_pid" 2>/dev/null
  return $rc
}

# ── dates and stat ──────────────────────────────────────────────────────────
ab_now() { date +%s; }
# ISO-8601 with offset; BSD date has no -Is
ab_iso_date() { date +%Y-%m-%dT%H:%M:%S%z; }

ab_stat_mtime() { # seconds since epoch, 0 if missing
  if [ "$AB_OS" = darwin ]; then stat -f %m "$1" 2>/dev/null || echo 0
  else stat -c %Y "$1" 2>/dev/null || echo 0; fi
}
ab_stat_size() {
  if [ "$AB_OS" = darwin ]; then stat -f %z "$1" 2>/dev/null || echo 0
  else stat -c %s "$1" 2>/dev/null || echo 0; fi
}
ab_stat_btime() { # birth time, falls back to mtime when unknown
  local t
  if [ "$AB_OS" = darwin ]; then t=$(stat -f %B "$1" 2>/dev/null)
  else t=$(stat -c %W "$1" 2>/dev/null); fi
  if [ -z "$t" ] || [ "$t" = 0 ] || [ "$t" = "-" ]; then t=$(ab_stat_mtime "$1"); fi
  printf '%s' "${t:-0}"
}

# ── processes: /proc does not exist on macOS ────────────────────────────────
ab_ppid() { # parent pid of $1, empty if gone
  local p="$1"
  [ -n "$p" ] || return 0
  if [ "$AB_OS" = linux ] && [ -r "/proc/$p/stat" ]; then
    # comm is parenthesized and may contain spaces: read the field after the last ")"
    sed -E 's/^.*\) [A-Za-z] ([0-9]+).*/\1/' "/proc/$p/stat" 2>/dev/null
  else
    ps -o ppid= -p "$p" 2>/dev/null | tr -d ' '
  fi
}

ab_cwd_of_pid() { # working directory of $1
  local p="$1"
  [ -n "$p" ] || return 0
  if [ "$AB_OS" = linux ] && [ -e "/proc/$p/cwd" ]; then
    readlink -f "/proc/$p/cwd" 2>/dev/null
  else
    lsof -a -p "$p" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1
  fi
}

ab_files_of_pid() { # regular files the process has open, one per line
  local p="$1"
  [ -n "$p" ] || return 0
  if [ "$AB_OS" = linux ] && [ -d "/proc/$p/fd" ]; then
    ls -l "/proc/$p/fd" 2>/dev/null | sed -n 's/.* -> //p'
  else
    lsof -a -p "$p" -d 0-255 -Fn 2>/dev/null | sed -n 's/^n//p'
  fi
}

ab_pids_of() { # pids of processes whose executable basename is $1, one per line
  # BSD pgrep -x matches the whole executable path, so an app or a binary invoked
  # by absolute path never matches by name: compare basenames through ps instead.
  if [ "$AB_OS" = linux ]; then
    pgrep -x "$1" 2>/dev/null
  else
    ps -Ao pid=,comm= | awk -v n="$1" '{ p=$1; sub(/^[ ]*[0-9]+[ ]+/,""); c=$0; sub(/.*\//,"",c); if (c==n) print p }'
  fi
  return 0
}

ab_tty_of_pid() { # /dev/ttys003 style, empty when not on a tty
  local t
  t=$(ps -o tty= -p "$1" 2>/dev/null | tr -d ' ')
  case "$t" in
    ""|"?"|"??"|"-") return 0 ;;
    /dev/*) printf '%s' "$t" ;;
    *) printf '/dev/%s' "$t" ;;
  esac
}

# ── desktop notifications ───────────────────────────────────────────────────
# ab_load_config — the resolved ~/.config/airboss/config.toml as environment
# variables, via `airboss-tui config`, so the shell side never parses TOML and
# config.toml stays the single source of truth. Runs once per process; if the
# binary is missing the built-in defaults below stand.
ab_load_config() {
  [ -n "${AIRBOSS_CONFIG_LOADED:-}" ] && return 0
  AIRBOSS_CONFIG_LOADED=1
  local bin out
  bin="${AIRBOSS_TUI_BIN:-$HOME/.local/bin/airboss-tui}"
  if [ -x "$bin" ] && out=$("$bin" config 2>/dev/null) && [ -n "$out" ]; then
    eval "$out"
  fi
  : "${AIRBOSS_NOTIFY_ENABLED:=1}"
  : "${AIRBOSS_NOTIFY_SINKS:=desktop}"
  : "${AIRBOSS_NOTIFY_WAITING:=1}"
  : "${AIRBOSS_NOTIFY_ERROR:=1}"
  : "${AIRBOSS_NOTIFY_TURN_DONE:=1}"
  : "${AIRBOSS_NOTIFY_SESSION_DONE:=1}"
  : "${AIRBOSS_FEATURE_CLASSIFIER:=1}"
  : "${AIRBOSS_FEATURE_WINDOW_FOCUS:=1}"
  : "${AIRBOSS_FEATURE_APPLY_TITLE:=1}"
  : "${AIRBOSS_FEATURE_SYNC:=1}"
  return 0
}

# ab_feature <name> — true when [features].<name> is on. Names are the config
# keys: classifier, window_focus, apply_title, sync.
ab_feature() {
  ab_load_config
  local v
  case "$1" in
    classifier)   v="$AIRBOSS_FEATURE_CLASSIFIER" ;;
    window_focus) v="$AIRBOSS_FEATURE_WINDOW_FOCUS" ;;
    apply_title)  v="$AIRBOSS_FEATURE_APPLY_TITLE" ;;
    sync)         v="$AIRBOSS_FEATURE_SYNC" ;;
    *)            v=1 ;;
  esac
  [ "$v" = 1 ]
}

# ab_notify_event <event: waiting|error|turn_done|session_done> <urgency> <icon> <title> <body>
# Drops the notification when [notify.events].<event> is off.
ab_notify_event() {
  ab_load_config
  local ev="$1" on
  case "$ev" in
    waiting)      on="$AIRBOSS_NOTIFY_WAITING" ;;
    error)        on="$AIRBOSS_NOTIFY_ERROR" ;;
    turn_done)    on="$AIRBOSS_NOTIFY_TURN_DONE" ;;
    session_done) on="$AIRBOSS_NOTIFY_SESSION_DONE" ;;
    *)            on=1 ;;
  esac
  [ "$on" = 1 ] || return 0
  shift
  ab_notify "$@"
}

# ab_notify <urgency: low|normal|critical> <icon> <title> <body>
ab_notify() {
  local urgency="$1" icon="$2" title="$3" body="$4"
  "${AIRBOSS_NOTIFY_BIN:-$HOME/.local/bin/airboss-notify}" "$urgency" "$icon" "$title" "$body" 2>/dev/null
  return 0
}

# True when notifications are enabled and at least one sink is usable.
ab_can_notify() {
  ab_load_config
  [ "$AIRBOSS_NOTIFY_ENABLED" = 1 ] || return 1
  [ -n "$AIRBOSS_NOTIFY_SINKS" ] || return 1
  [ -x "${AIRBOSS_NOTIFY_BIN:-$HOME/.local/bin/airboss-notify}" ] && return 0
  command -v notify-send >/dev/null 2>&1
}
