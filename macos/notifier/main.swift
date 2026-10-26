// airboss-notifier — the macOS side of airboss-notify: posts one native
// notification through UserNotifications and, when it is clicked, focuses the
// terminal tab of the session behind it (airboss-focus). Lives inside
// Airboss.app so Notification Center shows its own name, icon and settings.
//
//   airboss-notifier -title T [-subtitle S] [-message M] [-sound NAME|none]
//                    [-level passive|active|time-sensitive] [-id ID]
//                    [-session FILE] [-pid PID]
//   airboss-notifier -remove ID        withdraws a delivered notification
//
// Exit: 0 posted, 2 notifications not allowed, 1 anything else.
// Launched with no arguments (macOS does this on a click) it only waits for the
// click to be delivered.
import AppKit
import UserNotifications

struct Options {
    var title = "airboss", subtitle = "", message = "", sound = "default"
    var level = "active", id = UUID().uuidString, session = "", pid = ""
    var remove = ""
}

func parse(_ args: [String]) -> Options? {
    guard !args.isEmpty else { return nil }
    var o = Options()
    var i = 0
    while i < args.count {
        let v = i + 1 < args.count ? args[i + 1] : ""
        switch args[i] {
        case "-title": o.title = v
        case "-subtitle": o.subtitle = v
        case "-message": o.message = v
        case "-sound": o.sound = v
        case "-level": o.level = v
        case "-id": o.id = v
        case "-session": o.session = v
        case "-pid": o.pid = v
        case "-remove": o.remove = v
        default:
            FileHandle.standardError.write("airboss-notifier: unknown option \(args[i])\n".data(using: .utf8)!)
            exit(1)
        }
        i += 2
    }
    return o
}

final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    let opts: Options?
    init(_ opts: Options?) { self.opts = opts }

    // the delegate must be in place before launch finishes, or the click that
    // launched us is never delivered
    func applicationWillFinishLaunching(_ n: Notification) {
        UNUserNotificationCenter.current().delegate = self
    }

    func applicationDidFinishLaunching(_ n: Notification) {
        guard let o = opts else {
            DispatchQueue.main.asyncAfter(deadline: .now() + 5) { exit(0) }
            return
        }
        if !o.remove.isEmpty {
            UNUserNotificationCenter.current().removeDeliveredNotifications(withIdentifiers: [o.remove])
            // the removal is asynchronous and has no completion handler
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { exit(0) }
            return
        }
        post(o)
    }

    func post(_ o: Options) {
        let center = UNUserNotificationCenter.current()
        center.requestAuthorization(options: [.alert, .sound]) { granted, _ in
            guard granted else { exit(2) }
            let c = UNMutableNotificationContent()
            c.title = o.title
            c.subtitle = o.subtitle
            c.body = o.message
            c.threadIdentifier = o.id
            c.userInfo = ["session": o.session, "pid": o.pid]
            switch o.sound {
            case "none", "": break
            case "default": c.sound = .default
            default: c.sound = UNNotificationSound(named: UNNotificationSoundName(o.sound))
            }
            switch o.level {
            case "passive": c.interruptionLevel = .passive
            case "time-sensitive": c.interruptionLevel = .timeSensitive
            default: c.interruptionLevel = .active
            }
            // same id = the new notification replaces the session's previous one
            let req = UNNotificationRequest(identifier: o.id, content: c, trigger: nil)
            center.add(req) { err in
                if let err { FileHandle.standardError.write("airboss-notifier: \(err)\n".data(using: .utf8)!) }
                exit(err == nil ? 0 : 1)
            }
        }
    }

    // shown even if Airboss.app happens to be frontmost
    func userNotificationCenter(_ c: UNUserNotificationCenter, willPresent n: UNNotification,
                                withCompletionHandler done: @escaping (UNNotificationPresentationOptions) -> Void) {
        done([.banner, .list, .sound])
    }

    func userNotificationCenter(_ c: UNUserNotificationCenter, didReceive r: UNNotificationResponse,
                                withCompletionHandler done: @escaping () -> Void) {
        let info = r.notification.request.content.userInfo
        if r.actionIdentifier == UNNotificationDefaultActionIdentifier {
            focus(session: info["session"] as? String ?? "", fallbackPid: info["pid"] as? String ?? "")
        }
        done()
        exit(0)
    }

    // pid and tmux pane are read at click time: the sync keeps them current
    func focus(session: String, fallbackPid: String) {
        var pid = fallbackPid, target = ""
        if !session.isEmpty, let data = FileManager.default.contents(atPath: session),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if let p = obj["pid"] as? Int, p > 0 { pid = String(p) }
            target = obj["tmux_target"] as? String ?? ""
        }
        guard !pid.isEmpty || !target.isEmpty else { return }
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "\(home)/.local/bin/airboss-focus")
        p.arguments = [pid, target]
        // a click launches us through launchd, with a bare PATH
        var env = ProcessInfo.processInfo.environment
        env["PATH"] = "\(home)/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
        p.environment = env
        try? p.run()
        p.waitUntilExit()
    }
}

let app = NSApplication.shared
// old launch paths still append a -psn_ process serial number
let delegate = AppDelegate(parse(CommandLine.arguments.dropFirst().filter { !$0.hasPrefix("-psn_") }))
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
