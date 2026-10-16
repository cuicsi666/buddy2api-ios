import SwiftUI

@main
struct BuddyApp: App {
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
        .onChange(of: scenePhase) { phase in
            let gw = Gateway.shared
            let defaults = UserDefaults.standard

            if phase == .background {
                // 进入后台：若网关在运行且开启了保活，重新确认保活会话
                let wanted = (defaults.object(forKey: "keepAlive") as? Bool) ?? true
                if gw.running && wanted {
                    let strong = (defaults.object(forKey: "keepAliveStrong") as? Bool) ?? false
                    _ = KeepAlive.shared.start(strong: strong)
                }
            } else if phase == .active {
                // 回到前台：同步状态；若网关已不在（被系统回收/重启过）且开启了自动启动，则重新拉起
                gw.syncState()
                if gw.running {
                    let wanted = (defaults.object(forKey: "keepAlive") as? Bool) ?? true
                    if wanted && !KeepAlive.shared.running {
                        let strong = (defaults.object(forKey: "keepAliveStrong") as? Bool) ?? false
                        _ = KeepAlive.shared.start(strong: strong)
                    }
                } else {
                    let auto = (defaults.object(forKey: "autoStart") as? Bool) ?? true
                    if auto { gw.start() }
                }
            }
        }
    }
}
