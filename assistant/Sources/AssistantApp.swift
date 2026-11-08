import AppKit

// Two modes:
//   Smart Cal Assistant --fm-status   → prints Apple Intelligence availability
//   Smart Cal Assistant --fm          → one Apple Intelligence request (JSON on stdin, JSON on stdout)
//   (no flags)                        → background app that serves Siri / Shortcuts actions
@main
struct AssistantMain {
    static func main() {
        let args = CommandLine.arguments
        if args.contains("--fm-status") {
            print(FM.status())
            exit(0)
        }
        if args.contains("--fm") {
            FM.runCLI() // never returns
        }
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
