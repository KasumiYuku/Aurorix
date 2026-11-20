package plugin

import (
	"sync"
	"time"
)

// Rate 是单条指令的调用频率上限：最近 Window 内最多 Limit 次，零值表示不限。
type Rate struct {
	Limit  int
	Window time.Duration
}

// Enabled 报告这条限制是否生效。
func (r Rate) Enabled() bool { return r.Limit > 0 && r.Window > 0 }

// RateAllowed 报告这条指令现在能否再被这个用户调用；不能时返回还需等待多久。
func RateAllowed(commandPath, userID string, rate Rate) (bool, time.Duration) {
	return rateAllowedAt(commandPath, userID, rate, time.Now())
}

type clickRate struct {
	commandPath string
	rate        Rate
}

var (
	clickRateMu sync.RWMutex
	clickRates  = make(map[string]clickRate)
)

// BindClickRate 声明这个回调按钮属于哪条指令的限速: 点按钮与发指令消耗同一份额度; 不绑定则按钮不限速。
func BindClickRate(buttonID, commandPath string, rate Rate) {
	clickRateMu.Lock()
	defer clickRateMu.Unlock()
	clickRates[buttonID] = clickRate{commandPath: commandPath, rate: rate}
}

// ClickRateAllowed 报告这个按钮现在能否被这个用户点击; 不能时返回还需等待多久。
func ClickRateAllowed(buttonID, userID string) (bool, time.Duration) {
	clickRateMu.RLock()
	binding, ok := clickRates[buttonID]
	clickRateMu.RUnlock()
	if !ok {
		return true, 0
	}
	return RateAllowed(binding.commandPath, userID, binding.rate)
}

type rateBucket struct {
	hits []time.Time
	seen time.Time
}

const (
	rateIdleTTL   = 10 * time.Minute
	rateSweepEach = time.Minute
)

var (
	rateMu      sync.Mutex
	rateState   = make(map[string]*rateBucket)
	rateSweptAt time.Time
)

func rateAllowedAt(commandPath, userID string, rate Rate, now time.Time) (bool, time.Duration) {
	if !rate.Enabled() {
		return true, 0
	}
	key := commandPath + "\x00" + userID
	rateMu.Lock()
	defer rateMu.Unlock()
	sweepRates(now)
	bucket := rateState[key]
	if bucket == nil {
		bucket = &rateBucket{}
		rateState[key] = bucket
	}
	bucket.seen = now
	cut := now.Add(-rate.Window)
	kept := bucket.hits[:0]
	for _, at := range bucket.hits {
		if at.After(cut) {
			kept = append(kept, at)
		}
	}
	bucket.hits = kept
	if len(bucket.hits) >= rate.Limit {
		wait := bucket.hits[0].Add(rate.Window).Sub(now)
		if wait < 0 {
			wait = 0
		}
		return false, wait
	}
	bucket.hits = append(bucket.hits, now)
	return true, 0
}

func sweepRates(now time.Time) {
	if now.Sub(rateSweptAt) < rateSweepEach {
		return
	}
	rateSweptAt = now
	for key, bucket := range rateState {
		if now.Sub(bucket.seen) > rateIdleTTL {
			delete(rateState, key)
		}
	}
}
