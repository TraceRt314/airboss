#!/usr/bin/env python3
"""Render torre in a hidden tmux and export the screen as SVG (docs/screenshot.svg).
Usage: docs/screenshot.py out.svg [cols rows]"""
import re, subprocess, sys, time, html, os
out = sys.argv[1] if len(sys.argv) > 1 else "docs/screenshot.svg"
cols, rows = (int(sys.argv[2]), int(sys.argv[3])) if len(sys.argv) > 3 else (132, 30)
tui = os.path.expanduser("~/.local/bin/torre-tui")
subprocess.run(["tmux", "kill-session", "-t", "torre-shot"], stderr=subprocess.DEVNULL)
subprocess.run(["tmux", "new-session", "-d", "-s", "torre-shot", "-x", str(cols), "-y", str(rows), tui], check=True)
time.sleep(8)
raw = subprocess.run(["tmux", "capture-pane", "-p", "-e", "-t", "torre-shot"], capture_output=True, text=True).stdout
subprocess.run(["tmux", "kill-session", "-t", "torre-shot"])
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
