import Foundation
import Combine

/// 网关控制器：负责把 Go 内核（Buddy 2API）在 App 进程内拉起 / 停止。
///
/// 说明：Go 内核被编译成静态库（libbuddy.a）直接链接进 App，
/// 因此这不是「子进程」，服务与 App 同生命周期——App 被系统挂起时服务即停止。
final class Gateway: ObservableObject {
    static let shared = Gateway()

    @Published private(set) var running = false
    @Published private(set) var addr = ""
    @Published private(set) var busy = false
    @Published private(set) var keepAliveOn = false
    @Published var alert: String?

    /// 数据目录：Documents/Buddy2API（可在「文件」App 里直接查看、导入凭证）
    let dataDir: URL

    private init() {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        dataDir = docs.appendingPathComponent("Buddy2API", isDirectory: true)
        try? FileManager.default.createDirectory(at: dataDir, withIntermediateDirectories: true)
        syncState()
    }

    /// 内核版本号
    var version: String {
        guard let p = BuddyVersion() else { return "-" }
        defer { BuddyFree(p) }
        return String(cString: p)
    }

    /// 已运行时长（秒）
    var uptime: Int { Int(BuddyUptime()) }

    var panelURL: URL? {
        guard running, !addr.isEmpty else { return nil }
        return URL(string: "http://\(addr)")
    }

    func start() {
        guard !busy, !running else { return }
        busy = true
        let dir = dataDir.path
        let listen = "127.0.0.1:10082"
        DispatchQueue.global(qos: .userInitiated).async {
            let ret = dir.withCString { cDir in
                listen.withCString { cListen in
                    BuddyStart(UnsafeMutablePointer(mutating: cDir),
                               UnsafeMutablePointer(mutating: cListen))
                }
            }
            var err: String?
            if let p = ret {
                err = String(cString: p)
                BuddyFree(p)
            }
            DispatchQueue.main.async {
                self.busy = false
                if let e = err, !e.isEmpty {
                    self.alert = e
                }
                self.syncState()
                if self.running {
                    self.applyKeepAlive(UserDefaults.standard.object(forKey: "keepAlive") as? Bool ?? true)
                }
            }
        }
    }

    func stop() {
        guard !busy, running else { return }
        busy = true
        DispatchQueue.global(qos: .userInitiated).async {
            let ret = BuddyStop()
            var err: String?
            if let p = ret {
                err = String(cString: p)
                BuddyFree(p)
            }
            DispatchQueue.main.async {
                self.busy = false
                if let e = err, !e.isEmpty { self.alert = e }
                KeepAlive.shared.stop()
                self.keepAliveOn = false
                self.syncState()
            }
        }
    }

    /// 开关后台保活（静音音频播放，防系统挂起）
    func applyKeepAlive(_ wanted: Bool) {
        if wanted && running {
            keepAliveOn = KeepAlive.shared.start()
        } else {
            KeepAlive.shared.stop()
            keepAliveOn = false
        }
    }

    func syncState() {
        running = BuddyRunning() == 1
        if running, let p = BuddyAddr() {
            addr = String(cString: p)
            BuddyFree(p)
        } else {
            addr = ""
        }
    }

    /// 运行时长文本
    var uptimeText: String {
        let s = uptime
        if s < 60 { return "\(s) 秒" }
        if s < 3600 { return "\(s / 60) 分钟" }
        return "\(s / 3600) 小时 \((s % 3600) / 60) 分"
    }
}
