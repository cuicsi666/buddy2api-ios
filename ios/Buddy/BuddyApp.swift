import SwiftUI

@main
struct BuddyApp: App {
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
        .onChange(of: scenePhase) { phase in
            // 进入后台时：若网关在运行且开启了保活，重新确认保活音频会话
            if phase == .background {
                let gw = Gateway.shared
                let wanted = (UserDefaults.standard.object(forKey: "keepAlive") as? Bool) ?? true
                if gw.running && wanted {
                    _ = KeepAlive.shared.start()
                }
            }
        }
    }
}
