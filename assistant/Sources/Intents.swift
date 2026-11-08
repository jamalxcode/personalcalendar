import AppIntents
import Foundation

// Siri, Shortcuts and Spotlight actions. Each one asks the running Smart Cal (starting it if needed),
// which does the real work — same AI, same calendars, same default alerts as the menu bar app.

struct AddEventIntent: AppIntent {
    static var title: LocalizedStringResource = "Add to Smart Cal"
    static var description = IntentDescription("Add events by describing them, like “lunch with Ali Friday at 1”.")
    static var openAppWhenRun: Bool = false

    @Parameter(title: "Event", requestValueDialog: "What should I add?")
    var text: String

    static var parameterSummary: some ParameterSummary { Summary("Add \(\.$text) to Smart Cal") }

    func perform() async throws -> some IntentResult & ReturnsValue<String> & ProvidesDialog {
        let reply = try await Bridge.send("add", text)
        return .result(value: reply, dialog: "\(reply)")
    }
}

struct AskSmartCalIntent: AppIntent {
    static var title: LocalizedStringResource = "Ask Smart Cal"
    static var description = IntentDescription("Ask about your schedule or change it, like “what's on Friday?” or “move lunch to 3”.")
    static var openAppWhenRun: Bool = false

    @Parameter(title: "Request", requestValueDialog: "What would you like to know or change?")
    var text: String

    static var parameterSummary: some ParameterSummary { Summary("Ask Smart Cal \(\.$text)") }

    func perform() async throws -> some IntentResult & ReturnsValue<String> & ProvidesDialog {
        let reply = try await Bridge.send("ask", text)
        return .result(value: reply, dialog: "\(reply)")
    }
}

struct NextEventIntent: AppIntent {
    static var title: LocalizedStringResource = "What's Next in Smart Cal"
    static var description = IntentDescription("Tells you your next event.")
    static var openAppWhenRun: Bool = false

    func perform() async throws -> some IntentResult & ReturnsValue<String> & ProvidesDialog {
        let reply = try await Bridge.send("next", "")
        return .result(value: reply, dialog: "\(reply)")
    }
}

struct SmartCalShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(intent: AddEventIntent(),
                    phrases: ["Add an event in \(.applicationName)", "Add to \(.applicationName)",
                              "New event in \(.applicationName)", "Add something to \(.applicationName)"],
                    shortTitle: "Add Event", systemImageName: "calendar.badge.plus")
        AppShortcut(intent: NextEventIntent(),
                    phrases: ["What's next in \(.applicationName)", "What's my next event in \(.applicationName)",
                              "Next event in \(.applicationName)"],
                    shortTitle: "What's Next", systemImageName: "clock")
        AppShortcut(intent: AskSmartCalIntent(),
                    phrases: ["Ask \(.applicationName)", "Check \(.applicationName)", "Change something in \(.applicationName)"],
                    shortTitle: "Ask Smart Cal", systemImageName: "sparkles")
    }
}
