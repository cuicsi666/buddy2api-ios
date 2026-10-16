# Buddy网关 (Buddy 2API for iOS)

把 **Buddy 2API**（WorkBuddy / CodeBuddy → OpenAI 兼容网关）搬进 iOS App：Go 内核以静态库形式链接进 App，
在 App 进程内启动 HTTP 服务（`127.0.0.1:10082`），原版 Web 管理面板用 WKWebView 直接复用。

- 上游内核：<https://github.com/EchoPing07/Buddy-2API-Go>（MIT）
- 参考模块：`buddy2api-magisk-v0.2.5-multi.10`（Android Magisk/KernelSU 模块，仅内核同源）

## 功能

| 项 | 说明 |
| --- | --- |
| OpenAI 兼容接口 | `POST /v1/chat/completions`（流式/非流式）、`GET /v1/models` |
| 管理面板 | 原版 Web 后台：OAuth 登录、API Key、官方余额、每日签到、成长任务、请求日志、仪表盘 |
| 数据目录 | `App/Documents/Buddy2API`（文件 App 可见，可直接导入 `token.json`） |
| 同机互访 | 本机其它 App（含 Minis）可直接访问 `http://127.0.0.1:10082/v1` |

## ⚠️ iOS 限制（与 Android 版的核心差别）

1. **无法后台常驻**：iOS 会在 App 进入后台后挂起进程，网关随之停止（Android Magisk 模块的「开机自启 + 后台常驻」在 iOS 上不可实现，除非越狱或使用网络扩展权限）。
2. **无 root 能力**：不做 hosts / iptables 劫持，只提供本地 HTTP 服务。
3. 单账号（上游开源版）；多账号池请用服务器端反代。

## 构建

GitHub Actions（`macos-14`）自动完成：Go 交叉编译 iOS 静态库 → xcodegen 生成工程 → xcodebuild → 打包 IPA。

```bash
# Go 内核 → iOS 静态库（macOS + Xcode）
cd go
SDK=$(xcrun --sdk iphoneos --show-sdk-path)
GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC=$(xcrun --sdk iphoneos -f clang) \
CGO_CFLAGS="-isysroot $SDK -miphoneos-version-min=15.0 -arch arm64" \
CGO_LDFLAGS="-isysroot $SDK -miphoneos-version-min=15.0 -arch arm64" \
go build -tags carchive -buildmode=c-archive -o ../ios/GoLib/libbuddy.a ./iosmain

# Xcode 工程
cd ../ios && xcodegen generate
xcodebuild -project Buddy2API.xcodeproj -scheme Buddy2API -configuration Release -sdk iphoneos build
```

## 目录结构

```
go/                 上游 Go 内核 + iOS 桥接
  buddy/bridge.go   纯 Go 桥接层（Start/Stop/Version…）
  iosmain/main.go   c-archive C 入口（//export BuddyStart …）
ios/                Xcode 工程（xcodegen project.yml + SwiftUI 源码）
tools/make_icon.py  图标生成脚本
.github/workflows/build.yml
```

## 使用

1. 安装 IPA（未签名，用全能签 / AltStore 自签）。
2. 打开 App → 自动启动网关 → 「打开管理面板」→ OAuth 登录 WorkBuddy / CodeBuddy 账号。
3. 面板里创建 API Key；客户端 `base_url` 填 `http://127.0.0.1:10082/v1`。

## 声明

仅用于学习与研究。上游内核 MIT，界面与逻辑版权归原作者所有。
