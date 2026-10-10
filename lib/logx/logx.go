// Package logx 进程内日志内核: 同一份记录流向控制台与环形缓冲, 供管理台订阅。
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultRing = 2048

// Options Open 配置。
type Options struct {
	Ring  int
	Level string
}

type core struct {
	consoleLevel atomic.Int32
	ring         *ring
	total        atomic.Uint64
	errors       atomic.Uint64
	hub          *hub
	out          io.Writer
	outMu        sync.Mutex
	color        bool
}

var std = &core{
	ring: newRing(DefaultRing),
	hub:  newHub(),
	out:  os.Stderr,
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
	std.consoleLevel.Store(int32(level))
	std.ring = std.ring.resized(opts.Ring)
	if f, ok := std.out.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			std.color = true
		}
	}
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
}

func (l *Logger) Debugf(format string, a ...any) { l.inner.Debug(fmt.Sprintf(format, a...)) }
func (l *Logger) Infof(format string, a ...any)  { l.inner.Info(fmt.Sprintf(format, a...)) }
func (l *Logger) Warnf(format string, a ...any)  { l.inner.Warn(fmt.Sprintf(format, a...)) }
func (l *Logger) Errorf(format string, a ...any) { l.inner.Error(fmt.Sprintf(format, a...)) }

type scopeHandler struct {
	scope string
}

func (h *scopeHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *scopeHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *scopeHandler) WithGroup(string) slog.Handler            { return h }

func (h *scopeHandler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{ID: int64(std.total.Add(1)), Time: r.Time.UnixMilli(), Level: r.Level.String(), Scope: h.scope, Msg: r.Message}
	std.ring.push(e)
	if r.Level >= slog.LevelError {
		std.errors.Add(1)
	}
	std.writeLine(e, r.Level)
	std.hub.broadcast(e)
	return nil
}

func (c *core) writeLine(e Entry, level slog.Level) {
	if level < slog.Level(c.consoleLevel.Load()) {
		return
	}
	var line strings.Builder
	line.Grow(len(e.Msg) + 48)
	t := time.UnixMilli(e.Time)
	line.WriteString(t.Format("2006-01-02 15:04:05.000"))
	if c.color {
		line.WriteString(" " + levelColor(level) + pad(e.Level, 5) + "\x1b[0m")
	} else {
		line.WriteString(" " + pad(e.Level, 5))
	}
	line.WriteString(" " + pad(e.Scope, 9))
	line.WriteString(" " + e.Msg + "\n")
	c.outMu.Lock()
	c.out.Write([]byte(line.String()))
	c.outMu.Unlock()
}

// SetConsoleLevel 热更控制台最低级别, 不影响缓冲内记录。
func SetConsoleLevel(level string) error {
	parsed, err := parseLevel(level)
	if err != nil {
		return err
	}
	std.consoleLevel.Store(int32(parsed))
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

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

var levelColors = map[slog.Level]string{
	slog.LevelDebug: "\x1b[90m",
	slog.LevelInfo:  "\x1b[37m",
	slog.LevelWarn:  "\x1b[33m",
	slog.LevelError: "\x1b[31m",
}

func levelColor(level slog.Level) string {
	if c, ok := levelColors[level]; ok {
		return c
	}
	return "\x1b[37m"
}
