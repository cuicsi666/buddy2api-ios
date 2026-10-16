import SwiftUI

struct ContentView: View {
    @ObservedObject private var gw = Gateway.shared
    @State private var showPanel = false
    @AppStorage("autoStart") private var autoStart = true
    @AppStorage("keepAlive") private var keepAlive = true

    private var alertBinding: Binding<Bool> {
        Binding(get: { gw.alert != nil }, set: { if !$0 { gw.alert = nil } })
    }

    var body: some View {
        NavigationView {
            List {
                statusSection
                controlSection
                panelSection
                infoSection
                tipSection
            }
            .listStyle(InsetGroupedListStyle())
            .navigationBarTitle("Buddy 2API", displayMode: .large)
            .sheet(isPresented: $showPanel) {
                if let url = gw.panelURL {
                    PanelSheet(url: url)
                }
            }
            .alert("提示", isPresented: alertBinding) {
                Button("好", role: .cancel) { gw.alert = nil }
            } message: {
                Text(gw.alert ?? "")
            }
            .onAppear {
                if autoStart && !gw.running { gw.start() }
            }
        }
        .navigationViewStyle(StackNavigationViewStyle())
    }

    // MARK: - 分区

    private var statusSection: some View {
        Section {
            HStack(spacing: 14) {
                ZStack {
                    Circle()
                        .fill(gw.running ? Color.green.opacity(0.16) : Color.gray.opacity(0.16))
                        .frame(width: 44, height: 44)
                    Circle()
                        .fill(gw.running ? Color.green : Color.gray)
                        .frame(width: 13, height: 13)
                }
                VStack(alignment: .leading, spacing: 3) {
                    Text(gw.running ? "网关运行中" : "网关已停止")
                        .font(.headline)
                    Text(gw.running ? "http://\(gw.addr)" : "点下方按钮启动")
                        .font(.caption)
                        .foregroundColor(.secondary)
                    if gw.running {
                        Text(gw.keepAliveOn ? "后台保活已开启" : "后台保活未开启")
                            .font(.caption2)
                            .foregroundColor(gw.keepAliveOn ? .green : .orange)
                    }
                }
                Spacer()
                if gw.busy { ProgressView() }
            }
            .padding(.vertical, 4)
        }
    }

    private var controlSection: some View {
        Section {
            Button(action: {
                if gw.running { gw.stop() } else { gw.start() }
            }) {
                HStack {
                    Image(systemName: gw.running ? "stop.circle.fill" : "play.circle.fill")
                    Text(gw.running ? "停止网关" : "启动网关")
                }
            }
            .disabled(gw.busy)

            Toggle("打开 App 时自动启动", isOn: $autoStart)

            Toggle("后台保活（静音音频）", isOn: $keepAlive)
                .onChange(of: keepAlive) { on in
                    gw.applyKeepAlive(on)
                }
        }
    }

    private var panelSection: some View {
        Section(header: Text("管理面板")) {
            Button(action: { showPanel = true }) {
                HStack {
                    Image(systemName: "safari")
                    Text("打开管理面板")
                }
            }
            .disabled(!gw.running)

            Text("面板与原版 Web 后台一致：OAuth 登录、API Key 管理、官方余额、每日签到、成长任务、请求日志。")
                .font(.caption)
                .foregroundColor(.secondary)
        }
    }

    private var infoSection: some View {
        Section(header: Text("信息")) {
            infoRow("内核版本", gw.version)
            infoRow("监听地址", gw.running ? gw.addr : "—")
            infoRow("运行时长", gw.running ? gw.uptimeText : "—")
            infoRow("后台保活", gw.keepAliveOn ? "已开启（静音音频）" : "未开启")
        }
    }

    private var tipSection: some View {
        Section(header: Text("使用提示")) {
            Text("· OpenAI 兼容接口：http://127.0.0.1:10082/v1（API Key 在面板中创建）")
            Text("· 数据目录：文件 App → 我的 iPhone → Buddy 2API → Buddy2API")
            Text("· iOS 会在 App 进入后台后挂起服务，需要持续使用时请让 App 保持前台")
            Text("· 同一台设备上的其它 App（含 Minis）可直接访问 127.0.0.1:10082")
        }
        .font(.footnote)
    }

    private func infoRow(_ title: String, _ value: String) -> some View {
        HStack {
            Text(title)
            Spacer()
            Text(value)
                .foregroundColor(.secondary)
        }
    }
}
