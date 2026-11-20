import Foundation

final class CloudMusicBeatPlugin: MyGoPlugin {
    private let work = DispatchQueue(label: "dev.egoist.cloudmusic.automix", qos: .utility)
    private var models: [URL: CloudMusicCoreMLModel] = [:]

    private func model(_ root: URL, _ name: String) throws -> CloudMusicCoreMLModel {
        let url = root.appendingPathComponent(name).appendingPathExtension("mlpackage")
        let model = try models[url] ?? CloudMusicCoreMLModel(package: url)
        models[url] = model
        return model
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        guard let package = args["Package"] as? String,
              let input = args["Input"] as? String, let frames = args["Frames"] as? Int else {
            reply(.failure(PluginFailure("Arguments", "Invalid Beat This request")))
            return
        }
        work.async { [self] in
            do {
                let data = try Data(contentsOf: URL(fileURLWithPath: input), options: .mappedIfSafe)
                let root = URL(fileURLWithPath: package, isDirectory: true)
                switch method {
                case "predict":
                    let prediction = try model(root, "BeatThis").beats(features: data, validFrames: frames)
                    reply(.success(["Beat": prediction.beat, "Downbeat": prediction.downbeat]))
                case "vocal":
                    reply(.success(try model(root, "FireRed").vocalActivity(features: data, frames: frames)))
                case "key":
                    guard let ends = args["Ends"] as? [Int] else { throw CloudMusicBeatModelError.invalidInput }
                    let vqt = try model(root, "SKeyFeatures")
                    let classifier = try model(root, "SKeyClassify")
                    reply(.success(try vqt.key(wave: data, frames: frames, chunkEnds: ends, classifier: classifier) {
                        FileManager.default.fileExists(atPath: input)
                    }))
                default:
                    reply(.failure(PluginFailure("NoMethod", method)))
                }
            } catch {
                reply(.failure(PluginFailure("Prediction", error.localizedDescription)))
            }
        }
    }
}
