// Package buddy 把 Buddy 2API 网关封装成可被 iOS App 直接调用的 Go API。
//
// 与命令行版（main.go）的差异：
//   - 无 signal / os.Exit：进程生命周期由 App 掌控，Start 返回后由 Stop 关闭；
//   - 日志同时写入数据目录下的 gateway.log，便于在 App 内查看；
//   - 监听地址可由调用方指定（写入 config.json 的 listen 字段）；
//   - 启动后就绪探测（dial 监听地址），确保 App 拿到「已可用」的确切时机。
//
// 本文件不含 cgo，可直接用于 gomobile bind；如需 c-archive 方式见 carchive.go。
package buddy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"buddy2api-go/internal/admin"
	"buddy2api-go/internal/apikey"
	"buddy2api-go/internal/auth"
	"buddy2api-go/internal/config"
	"buddy2api-go/internal/proxy"
	"buddy2api-go/internal/scheduler"
	"buddy2api-go/internal/store"
	"buddy2api-go/internal/upstream"
	"buddy2api-go/internal/web"
)

// DefaultListen 与上游命令行版保持一致。
const DefaultListen = "127.0.0.1:10082"

type instance struct {
	srv     *http.Server
	sched   *scheduler.Scheduler
	st      *store.Store
	logFile *os.File
	addr    string
	dataDir string
	startAt time.Time
}

var (
	mu   sync.Mutex
	inst *instance
)

// Version 返回内核版本号（构建时经 ldflags 注入）。
func Version() string { return proxy.Version }

// Running 返回网关是否正在运行。
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return inst != nil
}

// Addr 返回当前监听地址（未运行时为空串）。
func Addr() string {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return ""
	}
	return inst.addr
}

// Dir 返回当前数据目录（未运行时为空串）。
func Dir() string {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return ""
	}
	return inst.dataDir
}

// Uptime 返回已运行秒数（未运行时为 0）。
func Uptime() int {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return 0
	}
	return int(time.Since(inst.startAt).Seconds())
}

// Start 启动网关。dataDir 为数据目录（App 沙盒内），listen 形如 127.0.0.1:10082；
// listen 为空时沿用数据目录内 config.json 的设置（首次启动为 DefaultListen）。
func Start(dataDir, listen string) error {
	mu.Lock()
	defer mu.Unlock()
	if inst != nil {
		return fmt.Errorf("网关已在运行：%s", inst.addr)
	}
	a, err := boot(dataDir, listen)
	if err != nil {
		return err
	}
	inst = a
	return nil
}

// Stop 停止网关并释放资源；未运行时为空操作。
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return nil
	}
	a := inst
	inst = nil
	return a.shutdown()
}

func (a *instance) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		slog.Warn("关闭 HTTP 服务超时", "error", err)
	}
	a.sched.Stop()
	err := a.st.Close()
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
	slog.Info("网关已停止")
	return err
}

func boot(dataDir, listen string) (*instance, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, errors.New("dataDir 不能为空")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败：%w", err)
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("解析数据目录失败：%w", err)
	}

	// 日志：stdout + 数据目录内 gateway.log（App 内可查看）
	logFile, err := os.OpenFile(filepath.Join(abs, "gateway.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败：%w", err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile),
		&slog.HandlerOptions{Level: slog.LevelInfo})))

	slog.Info("启动 Buddy 2API 网关", "version", proxy.Version, "data_dir", abs)

	cfg, err := config.Load(abs)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败：%w", err)
	}
	if l := strings.TrimSpace(listen); l != "" {
		if err := config.ValidateListen(l); err != nil {
			return nil, err
		}
		if cfg.Get().Listen != l {
			if err := cfg.Update(func(c *config.Config) error { c.Listen = l; return nil }); err != nil {
				return nil, fmt.Errorf("保存监听地址失败：%w", err)
			}
		}
	}

	st, err := store.Open(filepath.Join(abs, "buddy2api.db"))
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败：%w", err)
	}
	toks, err := auth.NewTokenStore(abs)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("加载凭证失败：%w", err)
	}
	client := upstream.New(toks, func() string { return cfg.Get().Region }, cfg.Get().ChatTimeoutSeconds)
	models := upstream.NewModelCache(cfg.Get().Region, client)
	keys := apikey.New(st)
	sched := scheduler.New(cfg, client, st, models)
	session, err := admin.NewSession(abs)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("初始化会话失败：%w", err)
	}
	proxyH := proxy.New(client, models, keys, st, cfg)
	adminH := admin.New(cfg, st, toks, client, models, keys, sched, session)
	webH, err := web.New(proxy.Version)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("初始化前端失败：%w", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("X-Content-Type-Options", "nosniff"))
	r.Use(methodNotAllowedJSONBody)
	r.Use(requestLogger)

	r.Get("/health", proxyH.Health)

	r.Group(func(r chi.Router) {
		r.Use(keys.Authenticate)
		r.Post("/v1/chat/completions", proxyH.Chat)
		r.Get("/v1/models", proxyH.Models)
	})

	r.Route("/admin", adminH.Routes())

	r.Group(func(r chi.Router) {
		r.Use(skipCompressOnRange)
		r.Use(middleware.Compress(5, "text/html", "text/css", "text/javascript", "application/javascript", "application/json", "image/svg+xml"))
		webH.Mount(r)
	})
	r.NotFound(func(w http.ResponseWriter, req *http.Request) { web.NotFoundJSON(w) })

	addr := cfg.Get().Listen
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("HTTP 服务已启动", "listen", addr, "region", cfg.Get().Region)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "error", err)
		}
	}()

	// 就绪探测：最多等 10s（iOS 上沙盒内监听通常 <100ms 就绪）
	ready := false
	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		if portOpen(addr) {
			ready = true
			break
		}
	}
	if !ready {
		sched.Stop()
		_ = st.Close()
		_ = logFile.Close()
		return nil, fmt.Errorf("网关启动超时（未监听 %s）", addr)
	}
	slog.Info("网关就绪", "listen", addr)
	return &instance{srv: srv, sched: sched, st: st, logFile: logFile, addr: addr, dataDir: abs, startAt: time.Now()}, nil
}

// portOpen 探测本地监听是否就绪。
func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// ── 以下中间件与命令行版（main.go）保持同一语义，原样搬迁 ──

// skipCompressOnRange 让带 Range 的请求绕过压缩（206 与实体保持一致）。
func skipCompressOnRange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			r.Header.Del("Accept-Encoding")
		}
		next.ServeHTTP(w, r)
	})
}

// methodNotAllowedJSONBody 把 chi 默认 405 的空 body 换成全站统一 JSON 错误体。
func methodNotAllowedJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&methodNotAllowedJSONWriter{ResponseWriter: w}, r)
	})
}

func methodNotAllowedBody() string { return web.MethodNotAllowedBodyJSON() }

type methodNotAllowedJSONWriter struct {
	http.ResponseWriter
	is405       bool
	wroteHeader bool
}

func (m *methodNotAllowedJSONWriter) WriteHeader(code int) {
	if code == http.StatusMethodNotAllowed {
		if vals := m.Header().Values("Allow"); len(vals) > 1 {
			m.Header().Del("Allow")
			m.Header().Set("Allow", strings.Join(vals, ", "))
		}
		m.Header().Set("Content-Type", "application/json; charset=utf-8")
		m.is405 = true
		m.wroteHeader = true
		m.ResponseWriter.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = m.ResponseWriter.Write([]byte(methodNotAllowedBody()))
		return
	}
	m.writeHeaderOnce(code)
}

func (m *methodNotAllowedJSONWriter) writeHeaderOnce(code int) {
	if m.wroteHeader {
		return
	}
	m.wroteHeader = true
	m.ResponseWriter.WriteHeader(code)
}

func (m *methodNotAllowedJSONWriter) Write(p []byte) (int, error) {
	if m.is405 {
		return len(p), nil
	}
	m.writeHeaderOnce(http.StatusOK)
	return m.ResponseWriter.Write(p)
}

func (m *methodNotAllowedJSONWriter) Flush() {
	m.writeHeaderOnce(http.StatusOK)
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (m *methodNotAllowedJSONWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := m.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (m *methodNotAllowedJSONWriter) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := m.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(m.ResponseWriter, src)
}

func (m *methodNotAllowedJSONWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := m.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (m *methodNotAllowedJSONWriter) Unwrap() http.ResponseWriter { return m.ResponseWriter }

// requestLogger 极简访问日志（不记录正文）。
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"ms", time.Since(start).Milliseconds(),
			"bytes", ww.BytesWritten(),
		)
	})
}
