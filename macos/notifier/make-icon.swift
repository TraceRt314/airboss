// make-icon.swift <out.png> — draws the 1024 px Airboss app icon: a dark
// squircle with a mint-to-aqua glow and the "airplane" SF Symbol, the air boss
// of a carrier deck. Built at install time so no binary asset lives in git.
import AppKit

let size: CGFloat = 1024
let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "icon.png"
let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: Int(size), pixelsHigh: Int(size),
                           bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                           colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)

// Apple's icon grid: 824 px body centred on a 1024 canvas, ~185 px corners
let body = NSRect(x: 100, y: 100, width: 824, height: 824)
let shape = NSBezierPath(roundedRect: body, xRadius: 185, yRadius: 185)
NSGradient(colors: [NSColor(red: 0.07, green: 0.09, blue: 0.13, alpha: 1),
                    NSColor(red: 0.02, green: 0.03, blue: 0.05, alpha: 1)])!
    .draw(in: shape, angle: -90)

shape.addClip()
NSGradient(colors: [NSColor(red: 0.36, green: 0.95, blue: 0.78, alpha: 0.55),
                    NSColor(red: 0.20, green: 0.75, blue: 0.95, alpha: 0)])!
    .draw(fromCenter: NSPoint(x: 512, y: 470), radius: 0,
          toCenter: NSPoint(x: 512, y: 470), radius: 430, options: [])

let cfg = NSImage.SymbolConfiguration(pointSize: 470, weight: .semibold)
    .applying(.init(paletteColors: [.white]))
if let sym = NSImage(systemSymbolName: "airplane", accessibilityDescription: nil)?
    .withSymbolConfiguration(cfg) {
    let s = sym.size
    sym.draw(in: NSRect(x: 512 - s.width / 2, y: 500 - s.height / 2, width: s.width, height: s.height))
}
NSGraphicsContext.restoreGraphicsState()
try! rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: out))
