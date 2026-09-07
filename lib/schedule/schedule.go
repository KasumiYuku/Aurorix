package schedule

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"sync"
	"time"
)

var logger = logx.New("schedule")

// HandleFunc 定时任务处理函数
type HandleFunc func(ctx *context.ScheduleContext) error

// Job 定时任务定义
// Cron 与 Interval 二选一; 同时设置时优先 Interval
// 可预设推送目标 (GroupId / UserId / Target), 触发时自动写入 ScheduleContext
type Job struct {
	Id        string                 // 任务唯一 ID
	PluginId  string                 // 所属插件 ID
	Cron      string                 // 5 段 cron: 分 时 日 月 周, 如 "0 9 * * *" 每天 9:00
	Interval  time.Duration          // 固定间隔, 如 time.Hour
	Immediate bool                   // Interval 任务是否立即执行一次
	GroupId   string                 // 预设群 OpenID (可选)
	UserId    string                 // 预设用户 OpenID (可选)
	Target    constant.MessageOrigin // 预设发送目标; 未设时按 GroupId/UserId 推断
	Handle    HandleFunc
}

type registeredJob struct {
	job      *Job
	cron     *cronExpr
	lastFire time.Time // cron: 上次触发的分钟刻度
	paused   bool
	cancel   chan struct{}
	stopOnce sync.Once
}

func (rj *registeredJob) stop() {
	rj.stopOnce.Do(func() {
		close(rj.cancel)
	})
}

var (
	jobs       []*registeredJob
	jobsLock   sync.RWMutex
	client     *api.BotAPI
	stopCh     chan struct{}
	started    bool
	cronLoopOn bool
	startMu    sync.Mutex
)

// Register 注册定时任务 (插件 init 中调用, 风格同 buttons.RegisterCallbackFunc)
// 若 Id 已存在则覆盖旧任务 (旧任务会被取消)
func Register(job *Job) {
	if job == nil {
		logger.Warnf("忽略空 Job")
		return
	}
	if job.Id == "" {
		logger.Warnf("忽略无 Id 的 Job (plugin=%v)", job.PluginId)
		return
	}
	if job.Handle == nil {
		logger.Warnf("忽略无 Handle 的 Job: %v", job.Id)
		return
	}
	if job.Interval <= 0 && job.Cron == "" {
		logger.Warnf("忽略未设置 Cron/Interval 的 Job: %v", job.Id)
		return
	}

	rj := &registeredJob{
		job:    job,
		cancel: make(chan struct{}),
	}
	if job.Interval <= 0 {
		expr, err := parseCron(job.Cron)
		if err != nil {
			logger.Errorf("Job %v cron 解析失败: %v", job.Id, err)
			return
		}
		rj.cron = expr
	}

	jobsLock.Lock()
	replaced := false
	for i, existing := range jobs {
		if existing.job.Id == job.Id {
			logger.Warnf("Job Id=%v 已存在, 覆盖旧任务", job.Id)
			existing.stop()
			jobs[i] = rj
			replaced = true
			break
		}
	}
	if !replaced {
		jobs = append(jobs, rj)
	}
	jobsLock.Unlock()

	// 调度器已启动时, 为新任务拉起对应循环
	startMu.Lock()
	running := started
	needCron := running && job.Interval <= 0 && !cronLoopOn
	if needCron {
		cronLoopOn = true
	}
	startMu.Unlock()
	if !running {
		return
	}
	if job.Interval > 0 {
		go runInterval(rj)
	} else if needCron {
		go runCronLoop()
	}
	NotifyChanged()
}

// Cancel 取消并移除任务, 不可恢复. 返回是否找到该任务
func Cancel(id string) bool {
	jobsLock.Lock()
	found := false
	for i, rj := range jobs {
		if rj.job.Id == id {
			rj.stop()
			jobs = append(jobs[:i], jobs[i+1:]...)
			logger.Infof("已取消 Job %v", id)
			found = true
			break
		}
	}
	jobsLock.Unlock()
	if found {
		NotifyChanged()
	}
	return found
}

// Pause 暂停任务 (保留注册, 到点不触发). 返回是否找到该任务
func Pause(id string) bool {
	jobsLock.Lock()
	found := false
	for _, rj := range jobs {
		if rj.job.Id == id {
			rj.paused = true
			logger.Infof("已暂停 Job %v", id)
			found = true
			break
		}
	}
	jobsLock.Unlock()
	if found {
		NotifyChanged()
	}
	return found
}

// Resume 恢复已暂停的任务. 返回是否找到该任务
func Resume(id string) bool {
	jobsLock.Lock()
	found := false
	for _, rj := range jobs {
		if rj.job.Id == id {
			rj.paused = false
			logger.Infof("已恢复 Job %v", id)
			found = true
			break
		}
	}
	jobsLock.Unlock()
	if found {
		NotifyChanged()
	}
	return found
}

// IsPaused 查询任务是否暂停. exists 表示任务是否仍注册
func IsPaused(id string) (paused bool, exists bool) {
	jobsLock.RLock()
	defer jobsLock.RUnlock()
	for _, rj := range jobs {
		if rj.job.Id == id {
			return rj.paused, true
		}
	}
	return false, false
}

// Exists 任务是否仍注册 (未 Cancel)
func Exists(id string) bool {
	jobsLock.RLock()
	defer jobsLock.RUnlock()
	for _, rj := range jobs {
		if rj.job.Id == id {
			return true
		}
	}
	return false
}

// GetJobCount 已注册任务数 (含暂停中的)
func GetJobCount() int {
	jobsLock.RLock()
	defer jobsLock.RUnlock()
	return len(jobs)
}

// Start 启动调度器 (main 中在 qqapi 初始化后调用)
func Start(c *api.BotAPI) {
	startMu.Lock()
	defer startMu.Unlock()
	if started {
		logger.Infof("调度器已在运行")
		return
	}
	if c == nil {
		logger.Errorf("无法启动: api.BotAPI 为空")
		return
	}
	client = c
	stopCh = make(chan struct{})
	started = true

	jobsLock.RLock()
	snapshot := append([]*registeredJob(nil), jobs...)
	jobsLock.RUnlock()

	var intervalN, cronN int
	for _, rj := range snapshot {
		if rj.job.Interval > 0 {
			intervalN++
			go runInterval(rj)
		} else {
			cronN++
		}
	}
	if cronN > 0 {
		cronLoopOn = true
		go runCronLoop()
	}
	logger.Infof("调度器已启动 (interval=%d, cron=%d)", intervalN, cronN)
}

// Stop 停止整个调度器 (所有任务循环退出, 注册表保留)
func Stop() {
	startMu.Lock()
	defer startMu.Unlock()
	if !started {
		return
	}
	close(stopCh)
	started = false
	cronLoopOn = false
	logger.Infof("调度器已停止")
}

func runInterval(rj *registeredJob) {
	job := rj.job
	if job.Immediate {
		tryFire(rj)
	}
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-rj.cancel:
			return
		case <-ticker.C:
			tryFire(rj)
		}
	}
}

func runCronLoop() {
	// 对齐到下一整秒后每秒检查, 同一分钟只触发一次
	now := time.Now()
	time.Sleep(time.Until(now.Truncate(time.Second).Add(time.Second)))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case t := <-ticker.C:
			t = t.Local()
			minuteKey := t.Truncate(time.Minute)
			var toFire []*registeredJob
			jobsLock.Lock()
			for _, rj := range jobs {
				if rj.cron == nil || rj.paused {
					continue
				}
				if !rj.cron.match(t) {
					continue
				}
				if !rj.lastFire.IsZero() && rj.lastFire.Equal(minuteKey) {
					continue
				}
				rj.lastFire = minuteKey
				toFire = append(toFire, rj)
			}
			jobsLock.Unlock()
			for _, rj := range toFire {
				go fire(rj)
			}
			if len(toFire) > 0 {
				NotifyChanged()
			}
		}
	}
}

func tryFire(rj *registeredJob) {
	jobsLock.RLock()
	if rj.paused {
		jobsLock.RUnlock()
		return
	}
	// 已被 Cancel 时 cancel channel 已关闭
	select {
	case <-rj.cancel:
		jobsLock.RUnlock()
		return
	default:
	}
	rj.lastFire = time.Now()
	jobsLock.RUnlock()
	NotifyChanged()
	fire(rj)
}

func fire(rj *registeredJob) {
	job := rj.job
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Job %v (plugin=%v) panic: %v", job.Id, job.PluginId, r)
		}
	}()

	// 插件停用后其定时任务不再触发
	if job.PluginId != "" && !plugin.Enabled(job.PluginId) {
		return
	}
	ctx := &context.ScheduleContext{}
	ctx.Init(client)
	ctx.JobId = job.Id
	ctx.PluginId = job.PluginId
	ctx.MessageManager.PluginId = job.PluginId
	if job.PluginId != "" {
		ctx.BindStorage(job.PluginId, "schedule:"+job.Id)
	}
	applyJobTarget(ctx, job)

	logger.Infof("触发 Job %v (plugin=%v)", job.Id, job.PluginId)
	if err := job.Handle(ctx); err != nil {
		logger.Errorf("Job %v (plugin=%v) error: %v", job.Id, job.PluginId, err)
	}
}

// 将 Job 上预设的推送目标写入上下文
// Target 为 PrivateMessage 时强制私聊; 否则: 有 GroupId 走群, 仅有 UserId 走私聊
func applyJobTarget(ctx *context.ScheduleContext, job *Job) {
	if job.GroupId != "" {
		ctx.SetGroupId(job.GroupId)
	}
	if job.UserId != "" {
		ctx.SetUserId(job.UserId)
	}

	if job.Target == constant.PrivateMessage {
		ctx.SetMessageOrigin(constant.PrivateMessage)
		return
	}
	if job.GroupId != "" {
		ctx.SetMessageOrigin(constant.GroupMessage)
		return
	}
	if job.UserId != "" {
		ctx.SetMessageOrigin(constant.PrivateMessage)
	}
}

// JobInfo 任务管理视图。
type JobInfo struct {
	ID         string `json:"id"`
	PluginID   string `json:"plugin_id"`
	Kind       string `json:"kind"` // interval | cron
	Cron       string `json:"cron,omitempty"`
	IntervalMS int64  `json:"interval_ms,omitempty"`
	Immediate  bool   `json:"immediate"`
	Paused     bool   `json:"paused"`
	LastFire   int64  `json:"last_fire"` // Unix 毫秒, 0 表示尚未触发
	NextFire   int64  `json:"next_fire"` // 仅 cron 且未暂停时给出
}

// Jobs 快照当前任务列表。
func Jobs() []JobInfo {
	jobsLock.RLock()
	defer jobsLock.RUnlock()
	out := make([]JobInfo, 0, len(jobs))
	now := time.Now()
	for _, rj := range jobs {
		info := JobInfo{
			ID:        rj.job.Id,
			PluginID:  rj.job.PluginId,
			Immediate: rj.job.Immediate,
			Paused:    rj.paused,
		}
		if rj.job.Interval > 0 {
			info.Kind = "interval"
			info.IntervalMS = rj.job.Interval.Milliseconds()
		} else {
			info.Kind = "cron"
			info.Cron = rj.job.Cron
			if !rj.paused && rj.cron != nil {
				info.NextFire = nextCronFire(rj.cron, now).UnixMilli()
			}
		}
		if !rj.lastFire.IsZero() {
			info.LastFire = rj.lastFire.UnixMilli()
		}
		out = append(out, info)
	}
	return out
}

// nextCronFire 从 from 起找下一个命中时刻 (分钟粒度)。
func nextCronFire(expr *cronExpr, from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	for range 2880 { // 最多往后扫两天
		if expr.match(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}
