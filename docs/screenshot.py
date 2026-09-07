#!/usr/bin/env python3
"""Render airboss in a hidden tmux and export the screen as SVG (docs/screenshot.svg).
Usage: docs/screenshot.py out.svg [cols rows [airboss-tui flags...]]"""
import re, subprocess, sys, time, html, os
out = sys.argv[1] if len(sys.argv) > 1 else "docs/screenshot.svg"
cols, rows = (int(sys.argv[2]), int(sys.argv[3])) if len(sys.argv) > 3 else (132, 30)
extra = sys.argv[4:]  # flags for airboss-tui, e.g. -lang en -theme nord
tui = os.path.expanduser("~/.local/bin/airboss-tui")
here = os.path.dirname(os.path.abspath(__file__))
# fictional sessions from docs/demo so the screenshot never leaks real prompts
env = dict(os.environ, AGENT_BOARD_DIR=os.path.join(here, "demo"), AIRBOSS_NO_SYNC="1")
subprocess.run(["tmux", "kill-session", "-t", "airboss-shot"], stderr=subprocess.DEVNULL)
subprocess.run(["tmux", "new-session", "-d", "-s", "airboss-shot", "-x", str(cols), "-y", str(rows), " ".join([tui] + extra)], check=True, env=env)
time.sleep(8)
raw = subprocess.run(["tmux", "capture-pane", "-p", "-e", "-t", "airboss-shot"], capture_output=True, text=True).stdout
subprocess.run(["tmux", "kill-session", "-t", "airboss-shot"])
# the demo cards borrow a real pid so a window resolves; hide that window's real title
raw = re.sub(r"(ws \S+ · \S+ · )[^\x1b]*?(?=\s{2,}|\x1b)", r"\1✳ nebula-api/fix: rate limit behind proxy", raw)
bg = subprocess.run([tui, "color", "background"], capture_output=True, text=True).stdout.strip() or "#1e1e2e"
fg0 = subprocess.run([tui, "color", "foreground"], capture_output=True, text=True).stdout.strip() or "#cdd6f4"
CW, CH, PAD = 8.4, 19, 18
w, h = int(cols * CW + 2 * PAD), int(rows * CH + 2 * PAD)
svg = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" font-family="JetBrainsMono Nerd Font, JetBrains Mono, Fira Code, monospace" font-size="14">',
       f'<rect width="100%" height="100%" rx="12" fill="{bg}"/>']
sgr = re.compile(r'\x1b\[([0-9;]*)m')
for y, line in enumerate(raw.split("\n")[:rows]):
    fg, bgc, bold, x, pos = fg0, None, False, 0, 0
    parts = []
    for m in sgr.finditer(line):
        parts.append((line[pos:m.start()], fg, bgc, bold))
        pos = m.end()
        codes = [int(c) if c else 0 for c in m.group(1).split(";")]
        i = 0
        while i < len(codes):
            c = codes[i]
            if c == 0: fg, bgc, bold = fg0, None, False
            elif c == 1: bold = True
            elif c == 22: bold = False
            elif c == 39: fg = fg0
            elif c == 49: bgc = None
            elif c in (38, 48) and i + 4 < len(codes) and codes[i+1] == 2:
                col = "#%02x%02x%02x" % (codes[i+2], codes[i+3], codes[i+4])
                if c == 38: fg = col
                else: bgc = col
                i += 4
            i += 1
    parts.append((line[pos:], fg, bgc, bold))
    for text, f, b, bo in parts:
        if not text: continue
        n = len(text)
        if b: svg.append(f'<rect x="{PAD + x*CW:.1f}" y="{PAD + y*CH}" width="{n*CW:.1f}" height="{CH}" fill="{b}"/>')
        weight = ' font-weight="bold"' if bo else ''
        svg.append(f'<text x="{PAD + x*CW:.1f}" y="{PAD + y*CH + 14}" fill="{f}"{weight} xml:space="preserve">{html.escape(text)}</text>')
        x += n
svg.append("</svg>")
open(out, "w").write("\n".join(svg))
print(out, w, h)
