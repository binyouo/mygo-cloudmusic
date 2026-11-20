import CoreML
import Foundation

enum CloudMusicBeatModelError: LocalizedError {
    case invalidInput
    case invalidOutput

    var errorDescription: String? {
        switch self {
        case .invalidInput: return "Invalid Beat This feature window"
        case .invalidOutput: return "Invalid Beat This prediction"
        }
    }
}

final class CloudMusicCoreMLModel {
    private static let frames = 1500
    private static let bands = 128
    private let compiled: URL
    private let model: MLModel

    init(package: URL) throws {
        let compiled = try MLModel.compileModel(at: package)
        do {
            let config = MLModelConfiguration()
            // The next song may be analyzed while iOS is playing in the background.
            config.computeUnits = .cpuOnly
            model = try MLModel(contentsOf: compiled, configuration: config)
        } catch {
            try? FileManager.default.removeItem(at: compiled)
            throw error
        }
        self.compiled = compiled
    }

    deinit { try? FileManager.default.removeItem(at: compiled) }

    static func tensor(_ shape: [Int], bytes: Data) throws -> MLMultiArray {
        let array = try MLMultiArray(shape: shape.map { NSNumber(value: $0) }, dataType: .float32)
        guard bytes.count == array.count * MemoryLayout<Float>.size else {
            throw CloudMusicBeatModelError.invalidInput
        }
        bytes.withUnsafeBytes { buffer in
            if let source = buffer.baseAddress { array.dataPointer.copyMemory(from: source, byteCount: buffer.count) }
        }
        return array
    }

    func predict(_ features: [String: MLMultiArray]) throws -> MLFeatureProvider {
        let input = try MLDictionaryFeatureProvider(dictionary: features.mapValues { MLFeatureValue(multiArray: $0) })
        return try model.prediction(from: input)
    }

    func beats(features: Data, validFrames: Int) throws -> (beat: [Float], downbeat: [Float]) {
        guard (1...Self.frames).contains(validFrames) else { throw CloudMusicBeatModelError.invalidInput }
        let mel = try Self.tensor([1, Self.frames, Self.bands], bytes: features)
        let valid = try MLMultiArray(shape: [1, NSNumber(value: Self.frames)], dataType: .float32)
        let mask = valid.dataPointer.bindMemory(to: Float.self, capacity: Self.frames)
        for i in 0..<Self.frames { mask[i] = i < validFrames ? 1 : 0 }
        let output = try predict(["mel": mel, "valid": valid])
        guard let beat = output.featureValue(for: "beat")?.multiArrayValue, beat.count == Self.frames,
              let downbeat = output.featureValue(for: "downbeat")?.multiArrayValue, downbeat.count == Self.frames else {
            throw CloudMusicBeatModelError.invalidOutput
        }
        return ((0..<validFrames).map { beat[$0].floatValue }, (0..<validFrames).map { downbeat[$0].floatValue })
    }

    func vocalActivity(features: Data, frames: Int) throws -> [Float] {
        guard (2...3320).contains(frames) else { throw CloudMusicBeatModelError.invalidInput }
        let fbank = try Self.tensor([1, frames, 80], bytes: features)
        let output = try predict(["fbank": fbank])
        guard let p = output.featureValue(for: "probabilities")?.multiArrayValue, p.count == frames * 3 else {
            throw CloudMusicBeatModelError.invalidOutput
        }
        return (0..<frames).map { max(p[3 * $0].floatValue, p[3 * $0 + 1].floatValue) }
    }

    func key(wave: Data, frames: Int, chunkEnds: [Int], classifier: CloudMusicCoreMLModel, active: () -> Bool) throws -> Int {
        let rate = 22050, bins = 84, width = 44
        guard wave.count >= frames * MemoryLayout<Float>.size, chunkEnds.last == frames else {
            throw CloudMusicBeatModelError.invalidInput
        }
        var starts = [Int]()
        var previous = 0
        for end in chunkEnds {
            guard end >= previous, end <= frames else { throw CloudMusicBeatModelError.invalidInput }
            for i in 0..<((end - previous) / rate) { starts.append(previous + i * rate) }
            previous = end
        }
        let seconds = starts.count
        guard (3...1200).contains(seconds) else { throw CloudMusicBeatModelError.invalidInput }
        let channels = try MLMultiArray(shape: [1, 1, NSNumber(value: bins), NSNumber(value: width * seconds)], dataType: .float32)
        let values = channels.dataPointer.bindMemory(to: Float.self, capacity: channels.count)
        let block = rate * MemoryLayout<Float>.size
        for (second, start) in starts.enumerated() {
            guard active() else { throw CocoaError(.userCancelled) }
            try autoreleasepool {
                let offset = start * MemoryLayout<Float>.size
                let pcm = try Self.tensor([1, 1, rate], bytes: wave.subdata(in: offset..<(offset + block)))
                let features = try predict(["pcm": pcm])
                guard let chroma = features.featureValue(for: "chroma")?.multiArrayValue, chroma.count == bins * width else {
                    throw CloudMusicBeatModelError.invalidOutput
                }
                for bin in 0..<bins {
                    let start = (bin * seconds + second) * width
                    for i in 0..<width { values[start + i] = chroma[bin * width + i].floatValue }
                }
            }
        }
        let output = try classifier.predict(["chroma": channels])
        guard let keys = output.featureValue(for: "keys")?.multiArrayValue, keys.count == 24 else {
            throw CloudMusicBeatModelError.invalidOutput
        }
        var best = 0
        for i in 1..<keys.count where keys[i].floatValue > keys[best].floatValue { best = i }
        return best
    }
}
