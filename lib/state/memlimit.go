package state

import (
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
)

var (
	memLimitBytes atomic.Int64
	gcPercentVal  atomic.Int64
)

func init() {
	if cur := debug.SetMemoryLimit(-1); cur != math.MaxInt64 {
		memLimitBytes.Store(cur)
	}
	gcPercentVal.Store(100)
	if v, ok := os.LookupEnv("GOGC"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			gcPercentVal.Store(int64(n))
		} else if v == "off" {
			gcPercentVal.Store(-1)
		}
	}
}

const (
	autoLimitNum = 1
	autoLimitDen = 4
	minAutoLimit = 256 << 20
	maxAutoLimit = 1 << 30
)

// ApplyMemoryLimits 给 Go 运行时设堆内存软上限与 GC 目标, 返回一句给启动日志用的说明。
func ApplyMemoryLimits(limitMB, gcPercent int) string {
	var s string
	switch {
	case limitMB > 0:
		prev := debug.SetMemoryLimit(int64(limitMB) << 20)
		memLimitBytes.Store(int64(limitMB) << 20)
		s = fmt.Sprintf("GOMEMLIMIT=%dMiB(原 %s)", limitMB, humanBytes(prev))
	case memLimitBytes.Load() > 0:
		s = fmt.Sprintf("GOMEMLIMIT=%s(来自环境变量)", humanBytes(memLimitBytes.Load()))
	default:
		if auto := defaultMemLimit(); auto > 0 {
			debug.SetMemoryLimit(auto)
			memLimitBytes.Store(auto)
			s = fmt.Sprintf("GOMEMLIMIT=%s(按整机内存自动推导, 1/%d)", humanBytes(auto), autoLimitDen)
		} else {
			s = "GOMEMLIMIT=未设(读不到整机内存, 也没配)"
		}
	}

	if gcPercent != 0 {
		prev := debug.SetGCPercent(gcPercent)
		gcPercentVal.Store(int64(gcPercent))
		s += fmt.Sprintf(", GOGC=%d(原 %d)", gcPercent, prev)
	} else {
		s += fmt.Sprintf(", GOGC=%d", gcPercentVal.Load())
	}
	return s
}

// MemLimit 当前生效的堆内存软上限(字节); 0 表示未设。
func MemLimit() int64 { return memLimitBytes.Load() }

// GCPercent 当前生效的 GOGC; -1 表示 GC 已关闭。
func GCPercent() int { return int(gcPercentVal.Load()) }

func defaultMemLimit() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	const prefix = "MemTotal:"
	var total int64
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		f := strings.Fields(line[len(prefix):])
		if len(f) == 0 {
			break
		}
		if kb, err := strconv.ParseInt(f[0], 10, 64); err == nil {
			total = kb * 1024
		}
		break
	}
	if total <= 0 {
		return 0
	}
	limit := total * autoLimitNum / autoLimitDen
	if limit < minAutoLimit {
		limit = minAutoLimit
	}
	if limit > maxAutoLimit {
		limit = maxAutoLimit
	}
	return limit
}

func humanBytes(n int64) string {
	if n <= 0 || n == math.MaxInt64 {
		return "未设"
	}
	return fmt.Sprintf("%.0fMiB", float64(n)/(1<<20))
}
