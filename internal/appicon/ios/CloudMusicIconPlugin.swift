import UIKit

// The home screen's icon, which the settings switch between the app's and
// the dark variant's files (CFBundleAlternateIcons).
final class CloudMusicIconPlugin: MyGoPlugin {
    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "current":
            reply(.success(["Name": UIApplication.shared.alternateIconName ?? ""]))
        case "set":
            guard UIApplication.shared.supportsAlternateIcons else {
                reply(.failure(PluginFailure("Unsupported", "This iOS cannot switch app icons.")))
                return
            }
            let name = args["Name"] as? String ?? ""
            UIApplication.shared.setAlternateIconName(name.isEmpty ? nil : name) { error in
                if let error {
                    reply(.failure(PluginFailure("SetFailed", error.localizedDescription)))
                } else {
                    reply(.success(nil))
                }
            }
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
        }
    }
}
