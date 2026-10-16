//go:build carchive

// 通过 -buildmode=c-archive 把网关编译成 iOS 可链接的静态库时使用的 C 入口。
//
// 构建（macOS + Xcode 环境）：
//
//	GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
//	CC="$(xcrun --sdk iphoneos -f clang)" \
//	CGO_CFLAGS="-isysroot $(xcrun --sdk iphoneos --show-sdk-path) -miphoneos-version-min=15.0 -arch arm64" \
//	CGO_LDFLAGS="-isysroot $(xcrun --sdk iphoneos --show-sdk-path) -miphoneos-version-min=15.0 -arch arm64" \
//	go build -tags carchive -buildmode=c-archive -o libbuddy.a .
//
// 产物：libbuddy.a + libbuddy.h，由 Xcode 工程链接。
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"buddy2api-go/buddy"
)

// BuddyStart 启动网关；成功返回 NULL，失败返回错误信息（须用 BuddyFree 释放）。
//
//export BuddyStart
func BuddyStart(cDataDir, cListen *C.char) *C.char {
	dataDir := C.GoString(cDataDir)
	listen := C.GoString(cListen)
	if err := buddy.Start(dataDir, listen); err != nil {
		return C.CString(err.Error())
	}
	return nil
}

// BuddyStop 停止网关；成功返回 NULL，失败返回错误信息。
//
//export BuddyStop
func BuddyStop() *C.char {
	if err := buddy.Stop(); err != nil {
		return C.CString(err.Error())
	}
	return nil
}

// BuddyVersion 返回内核版本号（须用 BuddyFree 释放）。
//
//export BuddyVersion
func BuddyVersion() *C.char { return C.CString(buddy.Version()) }

// BuddyAddr 返回监听地址（须用 BuddyFree 释放）。
//
//export BuddyAddr
func BuddyAddr() *C.char { return C.CString(buddy.Addr()) }

// BuddyRunning 返回 1/0。
//
//export BuddyRunning
func BuddyRunning() C.int {
	if buddy.Running() {
		return 1
	}
	return 0
}

// BuddyUptime 返回已运行秒数。
//
//export BuddyUptime
func BuddyUptime() C.int { return C.int(buddy.Uptime()) }

// BuddyFree 释放由本库返回的 C 字符串。
//
//export BuddyFree
func BuddyFree(p *C.char) {
	if p != nil {
		C.free(unsafe.Pointer(p))
	}
}

// main 不会被调用；c-archive 要求包为 main。
func main() {}
