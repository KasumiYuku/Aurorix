package message

import (
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// Sent 是一条(或一组)已经发出去的消息: 撤回需要的平台 ID 与发送时刻; 零值时刻表示不判 2 分钟时限。
type Sent struct {
	IDs []string
	At  time.Time
}

// IsEmpty 报告这批句柄里有没有可撤的东西。
func (s Sent) IsEmpty() bool {
	for _, id := range s.IDs {
		if id != "" {
			return false
		}
	}
	return true
}

const recallWindow = 2 * time.Minute

var recallGrace = 400 * time.Millisecond

// SetRecallGrace 设置"发完到撤回"之间的缓冲, <=0 表示不缓冲; 平台受理不等于用户看到, 留一段缓冲顺序才真正落到用户眼里。全局设置。
func SetRecallGrace(d time.Duration) {
	if d < 0 {
		d = 0
	}
	recallGrace = d
}

// RecallGrace 报当前的缓冲时长(给启动日志与用例用)。
func RecallGrace() time.Duration { return recallGrace }

// RecallLater 把一批旧消息的撤回排进后台并立刻返回; 超过 2 分钟的直接跳过, 失败只记一行警告。
func RecallLater(qapi *api.BotAPI, target constant.Target, olds ...Sent) {
	if qapi == nil {
		return
	}
	for _, old := range olds {
		if old.IsEmpty() {
			continue
		}
		job := recallJob{qapi: qapi, target: target, ids: old.IDs, at: old.At}
		scheduleRecall(job, recallGrace)
	}
}

type recallJob struct {
	qapi   *api.BotAPI
	target constant.Target
	ids    []string
	at     time.Time
}

var scheduleRecall = func(job recallJob, grace time.Duration) {
	time.AfterFunc(grace, func() { recall(job) })
}

func recall(job recallJob) {
	if !job.at.IsZero() && time.Since(job.at) > recallWindow {
		return
	}
	for _, id := range job.ids {
		if id == "" {
			continue
		}
		if err := recallInvoke(job.qapi, job.target, id); err != nil {
			logger.Warnf("撤回旧消息失败(不影响业务) id=%s err=%v", id, err)
		}
	}
}

var recallInvoke = func(qapi *api.BotAPI, target constant.Target, id string) error {
	return qapi.Session(target).Recall(id, true)
}
