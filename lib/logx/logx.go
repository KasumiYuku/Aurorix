// Package logx 进程内日志内核: 同一份记录流向控制台与环形缓冲, 供管理台订阅。
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const DefaultRing = 2048

// Options Open 配置。
type Options struct {
	Ring  int
	Level string
}

type core struct {
	level      *slog.LevelVar
	ring       *ring
	total      atomic.Uint64
	errors     atomic.Uint64
	hub        *hub
	outMu      sync.Mutex
	out        io.Writer
	handler    *slog.TextHandler
	handlerOut io.Writer
}

var std = &core{
	level: new(slog.LevelVar),
	ring:  newRing(DefaultRing),
	hub:   newHub(),
	out:   os.Stderr,
}

var (
	scopeMu   sync.RWMutex
	scopeLogs = make(map[string]*Logger)
)

// Open 在启动早期调用一次, 调整缓冲容量与控制台级别。
func Open(opts Options) {
	if opts.Ring <= 0 {
		opts.Ring = DefaultRing
	}
	level := slog.LevelInfo
	if opts.Level != "" {
		if parsed, err := parseLevel(opts.Level); err == nil {
			level = parsed
		}
	}
	std.level.Set(level)
	std.ring = std.ring.resized(opts.Ring)
}

// New 返回绑定 scope 的记录器, scope 用于管理台来源筛选。
func New(scope string) *Logger {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "system"
	}
	scopeMu.RLock()
	l, ok := scopeLogs[scope]
	scopeMu.RUnlock()
	if ok {
		return l
	}
	scopeMu.Lock()
	defer scopeMu.Unlock()
	if l, ok = scopeLogs[scope]; ok {
		return l
	}
	l = &Logger{inner: slog.New(&scopeHandler{scope: scope})}
	scopeLogs[scope] = l
	return l
}

// Logger 带来源的 printf 风格记录器。
type Logger struct {
	inner *slog.Logger
	trace string
}

// WithTrace 返回绑定了一次触发标识的副本; trace 会随这条记录的整条链路走, 控制台可按它筛。
func (l *Logger) WithTrace(trace string) *Logger {
	if trace == "" || l.trace == trace {
		return l
	}
	return &Logger{inner: l.inner, trace: trace}
}

func (l *Logger) Debugf(format string, a ...any) { l.emit(slog.LevelDebug, format, a...) }
func (l *Logger) Infof(format string, a ...any)  { l.emit(slog.LevelInfo, format, a...) }
func (l *Logger) Warnf(format string, a ...any)  { l.emit(slog.LevelWarn, format, a...) }
func (l *Logger) Errorf(format string, a ...any) { l.emit(slog.LevelError, format, a...) }

func (l *Logger) emit(level slog.Level, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if l.trace == "" {
		l.inner.Log(context.Background(), level, msg)
		return
	}
	l.inner.LogAttrs(context.Background(), level, msg, slog.String(attrTrace, l.trace))
}

const attrTrace = "trace"

var traceSeq atomic.Uint64

// NewTrace 为一次用户可见的触发派生短标识: 消息 id 尾巴 + 自增序号。
func NewTrace(sourceID string) string {
	tail := sourceID
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	if tail == "" {
		tail = "t"
	}
	return "m" + tail + "-" + strconv.FormatUint(traceSeq.Add(1), 10)
}

type scopeHandler struct {
	scope string
}

// Enabled 恒为 true: 缓冲里留住全部级别, 控制台该不该打印由 TextHandler 自己的 LevelVar 决定。
func (h *scopeHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *scopeHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *scopeHandler) WithGroup(string) slog.Handler            { return h }

func (h *scopeHandler) Handle(ctx context.Context, r slog.Record) error {
	group, user := extractContext(r.Message)
	e := Entry{ID: int64(std.total.Add(1)), Time: r.Time.UnixMilli(), Level: r.Level.String(), Scope: h.scope, Msg: r.Message, Group: group, User: user}
	var fields map[string]string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == attrTrace {
			e.Trace = a.Value.String()
			return true
		}
		if fields == nil {
			fields = make(map[string]string, r.NumAttrs())
		}
		fields[a.Key] = a.Value.String()
		return true
	})
	e.Fields = fields
	std.ring.push(e)
	if r.Level >= slog.LevelError {
		std.errors.Add(1)
	}
	std.console(ctx, r, h.scope)
	std.hub.broadcast(e)
	return nil
}

func (c *core) console(ctx context.Context, r slog.Record, scope string) {
	line := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	line.AddAttrs(slog.String("scope", scope))
	r.Attrs(func(a slog.Attr) bool {
		line.AddAttrs(a)
		return true
	})
	h := c.consoleHandler()
	if !h.Enabled(ctx, line.Level) {
		return
	}
	_ = h.Handle(ctx, line)
}

func (c *core) consoleHandler() *slog.TextHandler {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if c.handler == nil || c.handlerOut != c.out {
		c.handler = slog.NewTextHandler(&consoleWriter{c: c}, &slog.HandlerOptions{Level: c.level})
		c.handlerOut = c.out
	}
	return c.handler
}

type consoleWriter struct{ c *core }

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.c.outMu.Lock()
	defer w.c.outMu.Unlock()
	return w.c.out.Write(p)
}

// SetConsoleLevel 热更控制台最低级别, 不影响缓冲内记录。
func SetConsoleLevel(level string) error {
	parsed, err := parseLevel(level)
	if err != nil {
		return err
	}
	std.level.Set(parsed)
	return nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("未知日志级别 %q (可选 debug/info/warn/error)", s)
}
