import Foundation
import Speech

final class CloudMusicSpeechPlugin: MyGoPlugin {
    private struct Recognition {
        let id: UInt64
        let recognizer: SFSpeechRecognizer
        let task: SFSpeechRecognitionTask
        let reply: PluginReply
    }

    private var active: Recognition?

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "authorize":
            SFSpeechRecognizer.requestAuthorization { status in
                DispatchQueue.main.async {
                    guard status == .authorized else {
                        reply(.failure(PluginFailure("Unauthorized")))
                        return
                    }
                    guard SFSpeechRecognizer()?.supportsOnDeviceRecognition == true else {
                        reply(.failure(PluginFailure("Unsupported")))
                        return
                    }
                    reply(.success(nil))
                }
            }
        case "recognize":
            guard let id = (args["ID"] as? NSNumber)?.uint64Value,
                  let path = args["Path"] as? String else {
                reply(.failure(PluginFailure("InvalidArgument")))
                return
            }
            recognize(id, path, reply)
        case "cancel":
            if let id = (args["ID"] as? NSNumber)?.uint64Value, active?.id == id {
                cancelRecognition()
            }
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }

    private func recognize(_ id: UInt64, _ path: String, _ reply: @escaping PluginReply) {
        guard SFSpeechRecognizer.authorizationStatus() == .authorized else {
            reply(.failure(PluginFailure("Unauthorized")))
            return
        }
        guard let recognizer = SFSpeechRecognizer(), recognizer.supportsOnDeviceRecognition else {
            reply(.failure(PluginFailure("Unsupported")))
            return
        }
        guard recognizer.isAvailable else {
            reply(.failure(PluginFailure("Unavailable")))
            return
        }
        cancelRecognition()
        let request = SFSpeechURLRecognitionRequest(url: URL(fileURLWithPath: path))
        request.requiresOnDeviceRecognition = true
        request.shouldReportPartialResults = false
        request.addsPunctuation = true
        recognizer.queue = .main
        let task = recognizer.recognitionTask(with: request) { [weak self] result, error in
            DispatchQueue.main.async {
                guard let self, let active = self.active, active.id == id else { return }
                if let result, result.isFinal {
                    self.active = nil
                    active.reply(.success(Self.segments(result.bestTranscription)))
                } else if let error {
                    self.active = nil
                    let failure = error as NSError
                    // A pause or music-only interlude can contain audio but no recognizable speech.
                    if failure.domain == "kAFAssistantErrorDomain" && failure.code == 1110 {
                        active.reply(.success([] as [[String: Any]]))
                    } else {
                        active.reply(.failure(PluginFailure("RecognitionFailed", failure.localizedDescription)))
                    }
                }
            }
        }
        active = Recognition(id: id, recognizer: recognizer, task: task, reply: reply)
    }

    private func cancelRecognition() {
        if let previous = active {
            active = nil
            previous.task.cancel()
            previous.reply(.failure(PluginFailure("Canceled")))
        }
    }

    private static func segments(_ transcription: SFTranscription) -> [[String: Any]] {
        let words = transcription.segments
        let text = transcription.formattedString as NSString
        var result: [[String: Any]] = []
        var first = 0
        for i in words.indices {
            let word = words[i]
            let end = word.timestamp + word.duration
            let last = i == words.count - 1
            let nextStart = last ? end : words[i + 1].timestamp
            if !last && nextStart - end < 0.45 && end - words[first].timestamp < 6 {
                continue
            }
            let lower = words[first].substringRange.location
            let upper = last ? text.length : words[i + 1].substringRange.location
            if lower < upper && upper <= text.length && end > words[first].timestamp {
                result.append([
                    "Start": words[first].timestamp,
                    "End": end,
                    "Text": text.substring(with: NSRange(location: lower, length: upper - lower))
                        .trimmingCharacters(in: .whitespacesAndNewlines),
                ])
            }
            first = i + 1
        }
        return result
    }
}
