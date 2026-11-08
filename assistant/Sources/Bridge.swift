import AppKit
import Foundation

/// Talks to the running Smart Cal over a private Unix socket in its settings folder:
/// one JSON line in ({"cmd","text"}), one JSON line out ({"reply"} or {"error"}).
enum Bridge {
    enum Failure: LocalizedError {
        case notRunning, message(String)
        var errorDescription: String? {
            switch self {
            case .notRunning: return "Smart Cal isn't running. Open it from Applications and try again."
            case .message(let m): return m
            }
        }
    }

    static var socketPath: String {
        NSHomeDirectory() + "/Library/Application Support/PersonalCalendar/assistant.sock"
    }

    static func send(_ cmd: String, _ text: String) async throws -> String {
        do { return try request(cmd, text) } catch Failure.message(let m) { throw Failure.message(m) } catch {}
        // Smart Cal isn't running: start it in the background (this helper lives inside its bundle) and retry.
        let main = Bundle.main.bundleURL.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let cfg = NSWorkspace.OpenConfiguration()
        cfg.activates = false
        _ = try? await NSWorkspace.shared.openApplication(at: main, configuration: cfg)
        for _ in 0..<60 {
            try await Task.sleep(nanoseconds: 250_000_000)
            do { return try request(cmd, text) } catch Failure.message(let m) { throw Failure.message(m) } catch {}
        }
        throw Failure.notRunning
    }

    struct NotConnected: Error {}

    static func request(_ cmd: String, _ text: String) throws -> String {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw NotConnected() }
        defer { close(fd) }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let path = Array(socketPath.utf8CString)
        guard path.count <= MemoryLayout.size(ofValue: addr.sun_path) else { throw NotConnected() }
        withUnsafeMutableBytes(of: &addr.sun_path) { raw in
            for (i, c) in path.enumerated() { raw[i] = UInt8(bitPattern: c) }
        }
        let ok = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size)) }
        }
        guard ok == 0 else { throw NotConnected() }
        var tv = timeval(tv_sec: 85, tv_usec: 0)
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))

        var req = try JSONSerialization.data(withJSONObject: ["cmd": cmd, "text": text])
        req.append(0x0A)
        let sent = req.withUnsafeBytes { Darwin.write(fd, $0.baseAddress, req.count) }
        guard sent == req.count else { throw NotConnected() }

        var data = Data()
        var buf = [UInt8](repeating: 0, count: 16384)
        while true {
            let n = Darwin.read(fd, &buf, buf.count)
            if n <= 0 { break }
            data.append(contentsOf: buf[0..<n])
            if data.last == 0x0A { break }
        }
        guard !data.isEmpty, let obj = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw Failure.message("Smart Cal didn't answer in time.")
        }
        if let e = obj["error"] as? String, !e.isEmpty { throw Failure.message(e) }
        return obj["reply"] as? String ?? ""
    }
}
