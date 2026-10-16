import SwiftUI
import WebKit

/// WKWebView 包装：直接把原版 Web 管理面板嵌进来。
struct PanelView: UIViewRepresentable {
    let url: URL

    func makeUIView(context: Context) -> WKWebView {
        let conf = WKWebViewConfiguration()
        conf.allowsInlineMediaPlayback = true
        let wv = WKWebView(frame: .zero, configuration: conf)
        wv.allowsBackForwardNavigationGestures = true
        wv.load(URLRequest(url: url))
        return wv
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}
}

/// 以 sheet 形式呈现的面板（带导航栏与关闭按钮）。
struct PanelSheet: View {
    let url: URL
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationView {
            PanelView(url: url)
                .navigationBarTitle("Buddy 2API 面板", displayMode: .inline)
                .toolbar {
                    ToolbarItem(placement: .navigationBarTrailing) {
                        Button("完成") { dismiss() }
                    }
                }
        }
        .navigationViewStyle(StackNavigationViewStyle())
    }
}
