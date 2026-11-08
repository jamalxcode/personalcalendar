import Foundation
#if canImport(FoundationModels)
import FoundationModels
#endif

/// Apple Intelligence (on-device Foundation Models) for Smart Cal's AI box.
/// Smart Cal sends {"instructions", "prompt", "schema"} — schema is the same JSON Schema it sends to
/// OpenRouter — and gets back {"json": "<generated JSON>"} or {"error": "..."}.
enum FM {
    static func status() -> String {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *) {
            switch SystemLanguageModel.default.availability {
            case .available:
                return "available"
            case .unavailable(let reason):
                switch reason {
                case .deviceNotEligible: return "device-not-eligible"
                case .appleIntelligenceNotEnabled: return "not-enabled"
                case .modelNotReady: return "not-ready"
                @unknown default: return "unavailable"
                }
            @unknown default:
                return "unavailable"
            }
        }
        #endif
        return "needs-macos-26"
    }

    static func runCLI() -> Never {
        let input = FileHandle.standardInput.readDataToEndOfFile()
        Task {
            var out: [String: String] = [:]
            do {
                guard let req = try JSONSerialization.jsonObject(with: input) as? [String: Any] else {
                    throw FMError.message("bad request")
                }
                out["json"] = try await generate(
                    instructions: req["instructions"] as? String ?? "",
                    prompt: req["prompt"] as? String ?? "",
                    schema: req["schema"] as? [String: Any])
            } catch {
                out["error"] = describe(error)
            }
            let data = (try? JSONSerialization.data(withJSONObject: out)) ?? Data("{}".utf8)
            FileHandle.standardOutput.write(data)
            exit(0)
        }
        dispatchMain()
    }

    /// CI self-test: builds Apple's GenerationSchema from each JSON Schema on stdin (an array). Needs no model.
    static func checkSchemas() -> Never {
        let input = FileHandle.standardInput.readDataToEndOfFile()
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *) {
            guard let list = try? JSONSerialization.jsonObject(with: input) as? [[String: Any]] else {
                print("bad input"); exit(1)
            }
            for (i, s) in list.enumerated() {
                do {
                    _ = try GenerationSchema(root: convert(s, name: "Result"), dependencies: [])
                    print("schema \(i): ok")
                } catch {
                    print("schema \(i): \(error)"); exit(1)
                }
            }
            print("status: \(status())")
            exit(0)
        }
        #endif
        print("FoundationModels not available in this SDK/OS"); exit(0)
    }

    static func generate(instructions: String, prompt: String, schema: [String: Any]?) async throws -> String {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *) {
            guard case .available = SystemLanguageModel.default.availability else {
                throw FMError.message("Apple Intelligence isn't available (\(status()))")
            }
            let session = LanguageModelSession(instructions: instructions)
            let options = GenerationOptions(temperature: 0.2)
            if let schema {
                let root = convert(schema, name: "Result")
                let gs = try GenerationSchema(root: root, dependencies: [])
                let r = try await session.respond(to: prompt, schema: gs, options: options)
                return r.content.jsonString
            }
            let r = try await session.respond(to: prompt, options: options)
            return r.content
        }
        #endif
        throw FMError.message("Apple Intelligence needs macOS 26 or later")
    }

    #if canImport(FoundationModels)
    /// JSON Schema (the subset Smart Cal uses) → Foundation Models dynamic schema.
    @available(macOS 26.0, *)
    static func convert(_ s: [String: Any], name: String) -> DynamicGenerationSchema {
        let desc = s["description"] as? String
        switch s["type"] as? String ?? "string" {
        case "object":
            let props = s["properties"] as? [String: Any] ?? [:]
            // "kind" first so the model decides what it's doing before filling the details
            let keys = props.keys.sorted { a, b in a == "kind" ? true : (b == "kind" ? false : a < b) }
            let properties = keys.map { key -> DynamicGenerationSchema.Property in
                let ps = props[key] as? [String: Any] ?? [:]
                return DynamicGenerationSchema.Property(
                    name: key, description: ps["description"] as? String,
                    schema: convert(ps, name: name + "_" + key))
            }
            return DynamicGenerationSchema(name: name, description: desc, properties: properties)
        case "array":
            let items = s["items"] as? [String: Any] ?? ["type": "string"]
            return DynamicGenerationSchema(arrayOf: convert(items, name: name + "_item"))
        case "integer":
            return DynamicGenerationSchema(type: Int.self)
        case "number":
            return DynamicGenerationSchema(type: Double.self)
        case "boolean":
            return DynamicGenerationSchema(type: Bool.self)
        default:
            if let choices = s["enum"] as? [String], !choices.isEmpty {
                return DynamicGenerationSchema(name: name, description: desc, anyOf: choices)
            }
            return DynamicGenerationSchema(type: String.self)
        }
    }
    #endif

    static func describe(_ error: Error) -> String {
        if case FMError.message(let m) = error { return m }
        let s = String(describing: error)
        if s.contains("exceededContextWindowSize") { return "that was too long for Apple Intelligence" }
        if s.contains("guardrailViolation") { return "Apple Intelligence declined that request" }
        if s.contains("unsupportedLanguageOrLocale") { return "Apple Intelligence doesn't support that language yet" }
        return error.localizedDescription
    }
}

enum FMError: Error { case message(String) }
