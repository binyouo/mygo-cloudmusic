import AVFoundation
import CryptoKit
import MediaPlayer
import UIKit

// iOS uses the tall artwork for the full-screen lock-screen presentation.
// Render and encode only if the system requests it.
@available(iOS 26.0, *)
actor LockScreenArtwork {
    private static let size = CGSize(width: 720, height: 960)
    private let cover: UIImage
    private let file = FileManager.default.temporaryDirectory
        .appendingPathComponent(UUID().uuidString).appendingPathExtension("mp4")
    private var poster: UIImage?
    private var movie: Task<URL?, Never>?

    init(cover: UIImage) {
        self.cover = cover
    }

    deinit {
        try? FileManager.default.removeItem(at: file)
    }

    nonisolated static func nowPlayingInfo(image: UIImage, data: Data) -> [String: Any] {
        let key = MPNowPlayingInfoProperty3x4AnimatedArtwork
        guard MPNowPlayingInfoCenter.supportedAnimatedArtworkKeys.contains(key) else { return [:] }
        let source = LockScreenArtwork(cover: image)
        let artwork = MPMediaItemAnimatedArtwork(
            artworkID: "mygo-lock-screen-v1-\(SHA256.hash(data: data))",
            previewImageRequestHandler: { _ in await source.preview() },
            videoAssetFileURLRequestHandler: { _ in await source.video() }
        )
        return [key: artwork]
    }

    private func preview() -> UIImage {
        if let poster { return poster }
        let size = Self.size
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = true
        let rendered = UIGraphicsImageRenderer(size: size, format: format).image { output in
            let context = output.cgContext
            let colors = Self.edgeColors(of: cover)
            let space = CGColorSpaceCreateDeviceRGB()
            let backdrop = CGGradient(colorsSpace: space, colors: colors as CFArray, locations: nil)!
            context.drawLinearGradient(backdrop, start: .zero, end: CGPoint(x: 0, y: size.height), options: [])

            let scale = min(size.width / cover.size.width, size.height / cover.size.height)
            let width = cover.size.width * scale
            let height = cover.size.height * scale
            let rect = CGRect(x: (size.width - width) / 2, y: (size.height - height) / 2, width: width, height: height)
            context.beginTransparencyLayer(auxiliaryInfo: nil)
            cover.draw(in: rect)
            context.setBlendMode(.destinationIn)
            let fade = CGGradient(colorsSpace: space, colors: [
                UIColor.clear.cgColor, UIColor.white.cgColor,
                UIColor.white.cgColor, UIColor.clear.cgColor
            ] as CFArray, locations: [0, 0.10, 0.90, 1])!
            context.drawLinearGradient(fade, start: CGPoint(x: 0, y: rect.minY), end: CGPoint(x: 0, y: rect.maxY), options: [])
            context.endTransparencyLayer()
        }
        poster = rendered
        return rendered
    }

    private static func edgeColors(of image: UIImage) -> [CGColor] {
        guard let source = image.cgImage,
              let sample = CGContext(data: nil, width: 1, height: 2, bitsPerComponent: 8, bytesPerRow: 4,
                                     space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue),
              let pixels = sample.data?.assumingMemoryBound(to: UInt8.self) else {
            return [UIColor.black.cgColor, UIColor.black.cgColor]
        }
        sample.interpolationQuality = .high
        sample.draw(source, in: CGRect(x: 0, y: 0, width: 1, height: 2))
        return [0, 4].map { i in
            UIColor(red: CGFloat(pixels[i]) / 255, green: CGFloat(pixels[i + 1]) / 255,
                    blue: CGFloat(pixels[i + 2]) / 255, alpha: 1).cgColor
        }
    }

    private func video() async -> URL? {
        if let movie { return await movie.value }
        guard let image = preview().cgImage else { return nil }
        let destination = file
        let task = Task(priority: .utility) { await Self.writeMovie(image, to: destination) }
        movie = task
        return await task.value
    }

    // A short still loop gives iOS a video asset matching the preview.
    private static func writeMovie(_ image: CGImage, to file: URL) async -> URL? {
        let width = image.width, height = image.height
        var buffer: CVPixelBuffer?
        guard CVPixelBufferCreate(nil, width, height, kCVPixelFormatType_32BGRA, [
            kCVPixelBufferCGImageCompatibilityKey: true,
            kCVPixelBufferCGBitmapContextCompatibilityKey: true
        ] as CFDictionary, &buffer) == kCVReturnSuccess, let buffer else { return nil }
        CVPixelBufferLockBaseAddress(buffer, [])
        let bitmap = CGContext(data: CVPixelBufferGetBaseAddress(buffer), width: width, height: height,
                               bitsPerComponent: 8, bytesPerRow: CVPixelBufferGetBytesPerRow(buffer),
                               space: CGColorSpaceCreateDeviceRGB(),
                               bitmapInfo: CGBitmapInfo.byteOrder32Little.rawValue | CGImageAlphaInfo.noneSkipFirst.rawValue)
        bitmap?.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
        CVPixelBufferUnlockBaseAddress(buffer, [])
        guard bitmap != nil, let writer = try? AVAssetWriter(outputURL: file, fileType: .mp4) else { return nil }
        let input = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264, AVVideoWidthKey: width, AVVideoHeightKey: height
        ])
        guard writer.canAdd(input) else { return nil }
        writer.add(input)
        var format: CMVideoFormatDescription?
        guard CMVideoFormatDescriptionCreateForImageBuffer(allocator: nil, imageBuffer: buffer,
                                                            formatDescriptionOut: &format) == noErr,
              let format else { return nil }
        var timing = CMSampleTimingInfo(duration: CMTime(value: 2, timescale: 1),
                                        presentationTimeStamp: .zero, decodeTimeStamp: .invalid)
        var frame: CMSampleBuffer?
        guard CMSampleBufferCreateReadyWithImageBuffer(allocator: nil, imageBuffer: buffer,
                                                       formatDescription: format, sampleTiming: &timing,
                                                       sampleBufferOut: &frame) == noErr,
              let frame else { return nil }
        guard writer.startWriting() else { return nil }
        writer.startSession(atSourceTime: .zero)
        guard input.isReadyForMoreMediaData, input.append(frame) else {
            writer.cancelWriting()
            return nil
        }
        writer.endSession(atSourceTime: timing.duration)
        input.markAsFinished()
        await writer.finishWriting()
        return writer.status == .completed ? file : nil
    }
}
