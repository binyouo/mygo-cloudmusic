import AVFoundation
import MediaPlayer
import UIKit

final class CloudMusicPlaybackPlugin: MyGoPlugin {
    private let commands = MPRemoteCommandCenter.shared()
    private var interruption: NSObjectProtocol?
    private var routeChange: NSObjectProtocol?
    private var artwork: MPMediaItemArtwork?
    private var tallArtwork: [String: Any] = [:]
    private var loading = false
    private var loadTask = UIBackgroundTaskIdentifier.invalid

    init() {
        let session = AVAudioSession.sharedInstance()
        do {
            try session.setCategory(.playback, mode: .default, policy: .longFormAudio)
        } catch {
            NSLog("Cloud Music audio session: %@", error.localizedDescription)
        }

        add(commands.playCommand, "play")
        add(commands.pauseCommand, "pause")
        add(commands.togglePlayPauseCommand, "toggle")
        add(commands.previousTrackCommand, "previous")
        add(commands.nextTrackCommand, "next")
        commands.changePlaybackPositionCommand.addTarget { event in
            guard let position = (event as? MPChangePlaybackPositionCommandEvent)?.positionTime else {
                return .commandFailed
            }
            PluginHost.emit("cloudmusic_playback", "command", ["Action": "seek", "Position": position])
            return .success
        }
        enableCommands(false, seek: false)

        interruption = NotificationCenter.default.addObserver(
            forName: AVAudioSession.interruptionNotification, object: session, queue: .main
        ) { note in
            guard let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt,
                  let type = AVAudioSession.InterruptionType(rawValue: raw) else { return }
            switch type {
            case .began:
                let reason = note.userInfo?[AVAudioSessionInterruptionReasonKey] as? UInt
                // iOS also interrupts Now Playing when its route disconnects.
                Self.send(reason == AVAudioSession.InterruptionReason.routeDisconnected.rawValue
                    ? "routeDisconnected" : "interruptionBegan")
            case .ended:
                let rawOptions = note.userInfo?[AVAudioSessionInterruptionOptionKey] as? UInt ?? 0
                let shouldResume = AVAudioSession.InterruptionOptions(rawValue: rawOptions).contains(.shouldResume)
                PluginHost.emit("cloudmusic_playback", "command", [
                    "Action": "interruptionEnded", "ShouldResume": shouldResume
                ] as [String: Any])
            @unknown default:
                break
            }
        }
        routeChange = NotificationCenter.default.addObserver(
            forName: AVAudioSession.routeChangeNotification, object: session, queue: .main
        ) { note in
            guard let reason = note.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt,
                  reason == AVAudioSession.RouteChangeReason.oldDeviceUnavailable.rawValue else { return }
            Self.send("routeDisconnected")
        }
    }

    private static func send(_ action: String) {
        PluginHost.emit("cloudmusic_playback", "command", ["Action": action])
    }

    private func add(_ command: MPRemoteCommand, _ action: String) {
        command.addTarget { _ in
            Self.send(action)
            return .success
        }
    }

    private func enableCommands(_ enabled: Bool, seek: Bool) {
        commands.playCommand.isEnabled = enabled
        commands.pauseCommand.isEnabled = enabled
        commands.togglePlayPauseCommand.isEnabled = enabled
        commands.previousTrackCommand.isEnabled = enabled
        commands.nextTrackCommand.isEnabled = enabled
        commands.changePlaybackPositionCommand.isEnabled = seek
    }

    // Audio grants background time during playback. A bounded UIKit task keeps
    // a track change running through the gap before the next song has audio.
    private func setLoading(_ isLoading: Bool) {
        guard isLoading != loading else { return }
        loading = isLoading
        if isLoading {
            loadTask = UIApplication.shared.beginBackgroundTask(withName: "Load next track") { [weak self] in
                self?.endLoadTask()
            }
        } else {
            endLoadTask()
        }
    }

    private func endLoadTask() {
        guard loadTask != .invalid else { return }
        let task = loadTask
        loadTask = .invalid
        UIApplication.shared.endBackgroundTask(task)
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "update":
            setLoading(args["Loading"] as? Bool == true)
            let duration = args["Duration"] as? Double ?? 0
            let playing = args["Playing"] as? Bool == true
            if args["ArtworkChanged"] as? Bool == true {
                artwork = nil
                tallArtwork = [:]
                if let encoded = args["Artwork"] as? String,
                   let data = Data(base64Encoded: encoded), let image = UIImage(data: data) {
                    artwork = MPMediaItemArtwork(boundsSize: image.size) { size in
                        if size == image.size || size.width <= 0 || size.height <= 0 {
                            return image
                        }
                        return UIGraphicsImageRenderer(size: size).image { _ in
                            image.draw(in: CGRect(origin: .zero, size: size))
                        }
                    }
                    if #available(iOS 26.0, *) {
                        tallArtwork = LockScreenArtwork.nowPlayingInfo(image: image, data: data)
                    }
                }
            }
            var info: [String: Any] = [
                MPMediaItemPropertyTitle: args["Title"] as? String ?? "",
                MPMediaItemPropertyArtist: args["Artist"] as? String ?? "",
                MPMediaItemPropertyAlbumTitle: args["Album"] as? String ?? "",
                MPNowPlayingInfoPropertyMediaType: MPNowPlayingInfoMediaType.audio.rawValue,
                MPNowPlayingInfoPropertyElapsedPlaybackTime: args["Position"] as? Double ?? 0,
                MPNowPlayingInfoPropertyPlaybackRate: playing ? 1.0 : 0.0,
            ]
            if let trackID = args["TrackID"] { info[MPNowPlayingInfoPropertyExternalContentIdentifier] = trackID }
            if duration > 0 { info[MPMediaItemPropertyPlaybackDuration] = duration }
            if let artwork { info[MPMediaItemPropertyArtwork] = artwork }
            info.merge(tallArtwork) { _, artwork in artwork }
            enableCommands(true, seek: duration > 0)
            MPNowPlayingInfoCenter.default().nowPlayingInfo = info
            #if targetEnvironment(simulator)
            // Simulator needs explicit state to render the system's Now Playing UI.
            MPNowPlayingInfoCenter.default().playbackState = playing ? .playing : .paused
            #endif
        case "clear":
            setLoading(false)
            artwork = nil
            tallArtwork = [:]
            MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
            enableCommands(false, seek: false)
            #if targetEnvironment(simulator)
            MPNowPlayingInfoCenter.default().playbackState = .stopped
            #endif
        default:
            reply(.failure(PluginFailure("NoMethod", method)))
            return
        }
        reply(.success(nil))
    }
}
