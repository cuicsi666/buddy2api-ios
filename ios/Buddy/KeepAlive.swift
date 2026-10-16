import AVFoundation
import UIKit

/// 后台保活：借「音频」后台模式（UIBackgroundModes=audio），让 App 退到后台后不被系统挂起。
///
/// 原理：用 AVAudioSession(.playback) 持续播放一段**静音**音频。系统判定 App 正在播放音频，
/// 因此不会挂起进程 —— 本地 HTTP 服务（127.0.0.1:10082）得以在后台继续响应，
/// 同机其它 App（如 Minis）随时可以调用。
///
/// 说明：这是侧载（自签/企业签）场景下的常规做法，App Store 审核会拒绝此类用途。
/// 播放完全静音，不会发出声音；已设置 .mixWithOthers，不打断用户正在听的音乐。
final class KeepAlive {
    static let shared = KeepAlive()

    private let engine = AVAudioEngine()
    private let player = AVAudioPlayerNode()
    private var buffer: AVAudioPCMBuffer?
    private var attached = false
    private var observers: [NSObjectProtocol] = []

    private(set) var running = false

    private init() {}

    /// 开始保活（幂等）
    @discardableResult
    func start() -> Bool {
        if running {
            // 已开启：若引擎意外停了（被系统回收/打断），重新拉起
            if engine.isRunning { return true }
            running = false
        }
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.playback, mode: .default, options: [.mixWithOthers])
            try session.setActive(true)

            if !attached {
                engine.attach(player)
                guard let format = AVAudioFormat(standardFormatWithSampleRate: 8000, channels: 1) else {
                    return false
                }
                engine.connect(player, to: engine.mainMixerNode, format: format)
                let frames = AVAudioFrameCount(format.sampleRate) // 1 秒静音
                guard let buf = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: frames) else {
                    return false
                }
                buf.frameLength = frames // 未写入即全零 = 静音
                buffer = buf
                attached = true
            }

            try engine.start()
            if let buf = buffer {
                player.scheduleBuffer(buf, at: nil, options: [.loops], completionHandler: nil)
            }
            player.play()
            running = true
            registerObservers()
            return true
        } catch {
            running = false
            return false
        }
    }

    /// 停止保活
    func stop() {
        guard running else { return }
        player.stop()
        engine.stop()
        try? AVAudioSession.sharedInstance().setActive(false, options: [.notifyOthersOnDeactivation])
        running = false
        removeObservers()
    }

    private func registerObservers() {
        guard observers.isEmpty else { return }
        let c = NotificationCenter.default
        // 来电/其它音频打断结束后，重新拉起静音播放
        observers.append(c.addObserver(forName: AVAudioSession.interruptionNotification,
                                       object: nil, queue: .main) { [weak self] note in
            guard let self else { return }
            guard let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt,
                  let type = AVAudioSession.InterruptionType(rawValue: raw) else { return }
            if type == .ended {
                self.running = false
                _ = self.start()
            }
        })
        // 进入后台时确认引擎仍在跑
        observers.append(c.addObserver(forName: UIApplication.didEnterBackgroundNotification,
                                       object: nil, queue: .main) { [weak self] _ in
            guard let self else { return }
            if !self.engine.isRunning {
                self.running = false
                _ = self.start()
            }
        })
    }

    private func removeObservers() {
        observers.forEach { NotificationCenter.default.removeObserver($0) }
        observers.removeAll()
    }
}
