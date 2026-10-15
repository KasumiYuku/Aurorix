// Package quiz 是"分阶段给线索的抢答"运行时: 一桌一局、按时间表一层层放线索、全桌共用有限的提示、
// 谁都能开也谁都能收。
//
// 它不认识任何一道题、也不认识任何一句文案: 抽什么、提示有哪些、怎么判定、图怎么画由插件通过
// Source 给出, 三张卡长什么样由插件通过 Views 给出, 消息怎么发由 Sender 给出。本包只管节奏与闸门
// —— 那部分每个插件都一模一样, 写第二遍只会漂移。
package quiz

import (
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

var logger = logx.New("quiz")

// 三种"开不了局"与"没有局"的情形, 由插件翻成自己的话术。
var (
	ErrBusy       = errors.New("这个会话已经有一局在跑了")
	ErrNoRound    = errors.New("这个会话没有正在进行的对局")
	ErrNoQuestion = errors.New("抽不出题目")
)

// Kind 是这一张卡是什么。
type Kind uint8

const (
	// KindStage 是线索卡(谜面)。
	KindStage Kind = iota
	// KindHint 是提示卡。
	KindHint
	// KindOver 是收局卡(揭晓)。
	KindOver
	// KindNotice 是一句话的通知卡(猜错、收局了、提示用完了…)。
	KindNotice
)

// Stage 是一层线索: 到点放出来, 还原度与形态随层变。
type Stage struct {
	At time.Duration
	// Level 是这一层的还原度(0→1); 收局揭晓那张按 1 画。
	Level float64
	// Variant 是这一层的形态名; 空表示沿用这一局的形态(见 Round)。
	Variant string
}

// Rules 是一局的规矩; 零值会补齐成 DefaultRules 的那套。
type Rules struct {
	Stages []Stage
	// Lifetime 是自动收局的时刻。
	Lifetime time.Duration
	// HintLimit 是一局最多揭几条提示; HintCooldown 是两条之间的冷却(全桌共用)。
	HintLimit    int
	HintCooldown time.Duration
}

// DefaultRules 是五层、4 分 30 秒、三次提示、45 秒冷却的一套起手值。
func DefaultRules() Rules {
	return Rules{
		Stages: []Stage{
			{At: 0, Level: 0.06},
			{At: 50 * time.Second, Level: 0.24},
			{At: 105 * time.Second, Level: 0.46},
			{At: 160 * time.Second, Level: 0.62},
			{At: 230 * time.Second, Level: 0.80},
		},
		Lifetime:     270 * time.Second,
		HintLimit:    3,
		HintCooldown: 45 * time.Second,
	}
}

// Round 是一局的形态与答案。
type Round struct {
	Answer Answer
	// Variant 是这一局的形态名(插件自己的词, 如 "polar"); 只透传给 Draw 与 Views。
	Variant string
}

// Answer 是这一局要猜的东西; Data 由插件自己解释, 本包不碰。
type Answer struct {
	ID   string
	Data any
}

// Hint 一条提示: Label 是它揭的是什么, Text 是内容。
type Hint struct {
	Label string
	Text  string
}

// Task 是"出这一张图"的全部输入。
type Task struct {
	Answer  Answer
	Stage   int
	Stages  int
	Level   float64
	Variant string
	Final   bool
	Seed    int64
}

// Caption 是"这一张说些什么"的全部输入。
type Caption struct {
	Kind Kind
	Task
	Percent int
	Hint    Hint
	Left    int
	Head    string
	Reason  string
	// Elapsed 是这一局已经过去的时间; Used 是这一局揭过的提示(收局卡用)。
	Elapsed time.Duration
	Used    []Hint
}

// Views 是插件对卡面的排版权: 返回模板 id 与参数(按插件 id 填)。
type Views interface {
	Caption(c Caption) (string, templates.Args)
}

// Source 是插件给这一局的内容。
type Source interface {
	// Open 开局时抽一题并定下这一局的形态; ok=false 表示抽不出来。
	Open(rng *rand.Rand) (Round, bool)
	// Hints 报出这一题可给的提示, 外层是档: 越往后的档越贴脸。
	Hints(a Answer) [][]Hint
	// Judge 判定一次作答; hit 为真即猜中, notice 是要回给作答者的一句话(空 = 不回话)。
	Judge(query string, a Answer) (hit bool, notice string)
	// Draw 出一张图; t.Final 为真表示这是收局揭晓那张。
	Draw(t Task) (Image, error)
}

// Image 是一张要发出去的图; Key/Kind 给出图直链缓存用(没有就留空)。
type Image struct {
	Data []byte
	Key  string
	Kind string
}

// Outgoing 是一条要发出去的消息。
type Outgoing struct {
	Image Image
	// TemplateID / Args 是要发的正文: 本包只挑模板与参数, 填文本由出口做(见 NewSender)。
	TemplateID string
	Args       templates.Args
	// Summary 是平台侧的图片摘要(客户端图片没加载出来时显示它)。
	Summary  string
	Keyboard *buttons.Keyboard
	NoFooter bool
	// Mention 是要 @ 的人; 空表示不 @ 任何人 —— 定时放线索时就是这样。
	Mention string
}

// Sender 是一局的出口: 发一条消息并拿回平台消息 ID, 以及撤回一条。
type Sender interface {
	Post(m *context.MessageManager, out Outgoing) (string, error)
	Recall(m *context.MessageManager, messageID string) error
}

// Buttons 是三枚按钮的文案; 空则用默认值。Prefix 是那条指令的写法(如 "/p guess")。
type Buttons struct {
	Prefix  string
	Command string
	Guess   string
	Hint    string
	Stop    string
}

// Deps 是装配输入。
type Deps struct {
	// Id 是插件 id: 模板按它填, 按钮 id 按它加前缀(框架的按钮注册表是全局的)。
	Id     string
	Source Source
	Views  Views
	// Sender 为空时按框架默认方式发(见 NewSender)。
	Sender  Sender
	Buttons Buttons
	Rules   Rules
	// Jobs 是出图投递口(见 lib/plugin 的投递队列); 为空则当场画。
	Jobs interface{ Submit(func()) bool }
	// Clock 可注入(单测把时间拨快); 为空用系统时间。
	Clock func() time.Time
}

// Quiz 是一台机器上的全部局。
type Quiz struct {
	id      string
	source  Source
	views   Views
	sender  Sender
	buttons Buttons
	rules   Rules
	jobs    interface{ Submit(func()) bool }
	clock   func() time.Time

	mu     sync.Mutex
	rounds map[string]*session
}

type session struct {
	key     string
	manager *context.MessageManager
	answer  Answer
	variant string
	seed    int64
	stage   int
	message string
	hinted  int
	tier    int
	hintAt  time.Time
	used    []Hint
	started time.Time
	done    bool
	stop    chan struct{}
	once    sync.Once
}

// New 装配; Source 与 Views 缺一不可。
func New(d Deps) *Quiz {
	rules := d.Rules
	fallback := DefaultRules()
	if len(rules.Stages) == 0 {
		rules.Stages = fallback.Stages
	}
	if rules.Lifetime <= 0 {
		rules.Lifetime = fallback.Lifetime
	}
	if rules.HintLimit <= 0 {
		rules.HintLimit = fallback.HintLimit
	}
	if rules.HintCooldown <= 0 {
		rules.HintCooldown = fallback.HintCooldown
	}
	if d.Clock == nil {
		d.Clock = time.Now
	}
	if d.Sender == nil {
		d.Sender = NewSender(d.Id, nil)
	}
	return &Quiz{
		id: d.Id, source: d.Source, views: d.Views, sender: d.Sender,
		buttons: d.Buttons, rules: rules, jobs: d.Jobs, clock: d.Clock,
		rounds: map[string]*session{},
	}
}

// Playing 报告这个会话有没有一局在跑。
func (q *Quiz) Playing(group, user string) bool { return q.lookup(key(group, user)) != nil }

// HintButton 给出"查看提示"那枚按钮的 id 与回调 data —— 插件拿它注册回调。
func (q *Quiz) HintButton() (id, data string) { return q.id + ".hint", hintData }

// Keyboard 造线索卡底下那三枚: 两条快捷输入 + 一条回调。
func (q *Quiz) Keyboard() *buttons.Keyboard {
	prefix := q.buttons.Prefix
	if prefix == "" {
		prefix = "/" + q.buttons.Command
	}
	return buttons.New().Scope(q.id).
		Command("try", pick(q.buttons.Guess, "点击猜测"), "已填入", prefix+" ").
		Callback("hint", pick(q.buttons.Hint, "查看提示"), "已看提示", hintData).
		Command("stop", pick(q.buttons.Stop, "停止游戏"), "已填入", prefix+" stop").
		Build()
}

// Start 开一局: 抽题、摆第一层、然后按时间表推到收局。同一个会话同时只能有一局。
func (q *Quiz) Start(m *context.MessageManager) error {
	round, ok := q.source.Open(rand.New(rand.NewPCG(uint64(q.clock().UnixNano()), seedPattern)))
	if !ok {
		return ErrNoQuestion
	}
	s := &session{
		key:     key(m.GroupId, m.UserId),
		manager: m,
		answer:  round.Answer,
		variant: round.Variant,
		seed:    q.clock().UnixNano(),
		started: q.clock(),
		stop:    make(chan struct{}),
	}
	q.mu.Lock()
	if old := q.rounds[s.key]; old != nil && !old.done {
		q.mu.Unlock()
		return ErrBusy
	}
	q.rounds[s.key] = s
	q.mu.Unlock()

	q.advance(s)
	go q.run(s)
	return nil
}

// Guess 一次作答: 猜中就地收局, 其余按插件给的那句话回一声。
func (q *Quiz) Guess(m *context.MessageManager, query string) error {
	s := q.lookup(key(m.GroupId, m.UserId))
	if s == nil {
		return ErrNoRound
	}
	hit, notice := q.source.Judge(query, s.answer)
	if hit {
		q.finish(s)
		q.reveal(s, m, "猜中了", "")
		return nil
	}
	if notice == "" {
		return nil
	}
	return q.notice(m, notice)
}

// Stop 收局: 谁都能停 —— 一局占着整个会话, 不能让"谁能开"决定"谁能关"。
func (q *Quiz) Stop(m *context.MessageManager) error {
	s := q.lookup(key(m.GroupId, m.UserId))
	if s == nil {
		return q.notice(m, "这个会话没有正在进行的对局。")
	}
	q.finish(s)
	q.reveal(s, m, "这一局结束了", "有人喊停了这一局。")
	return nil
}

// Hint 揭一条提示: 次数与冷却都是全桌共用的, 谁点的都算。
func (q *Quiz) Hint(m *context.MessageManager) error {
	s := q.lookup(key(m.GroupId, m.UserId))
	if s == nil {
		return q.notice(m, "这一局已经结束了。")
	}
	now := q.clock()
	q.mu.Lock()
	switch {
	case s.hinted >= q.rules.HintLimit:
		q.mu.Unlock()
		return q.notice(m, "这一局的提示已经用完了。")
	case now.Sub(s.hintAt) < q.rules.HintCooldown:
		wait := q.rules.HintCooldown - now.Sub(s.hintAt)
		q.mu.Unlock()
		return q.notice(m, "刚给过提示，"+round(wait)+"之后再来。")
	}
	hint, ok := nextHint(q.source.Hints(s.answer), &s.tier)
	if !ok {
		q.mu.Unlock()
		return q.notice(m, "这一局没有更多提示可给了。")
	}
	s.hinted++
	s.hintAt = now
	s.used = append(s.used, hint)
	stage := s.stage
	left := q.rules.HintLimit - s.hinted
	q.mu.Unlock()

	task := q.task(s, stage, false)
	view := Caption{Kind: KindHint, Task: task, Percent: percent(task.Level), Hint: hint, Left: left}
	return q.send(s, task, view, m.UserId, false)
}

// run 是一局的节拍: 到点放下一层线索, 到点收局。
func (q *Quiz) run(s *session) {
	for i := 1; i < len(q.rules.Stages); i++ {
		if !wait(s, s.started.Add(q.rules.Stages[i].At)) {
			return
		}
		q.advance(s)
	}
	if !wait(s, s.started.Add(q.rules.Lifetime)) {
		return
	}
	q.finish(s)
	q.reveal(s, s.manager, "时间到", "")
}

// advance 放下一层线索(撤掉上一条)。
func (q *Quiz) advance(s *session) {
	q.mu.Lock()
	if s.done || s.stage >= len(q.rules.Stages) {
		q.mu.Unlock()
		return
	}
	s.stage++
	task := q.task(s, s.stage, false)
	q.mu.Unlock()

	if !q.submit(func() {
		q.send(s, task, Caption{Kind: KindStage, Task: task, Percent: percent(task.Level)}, "", true)
	}) {
		logger.Warnf("第 %d 层没排上出图队列, 这一层跳过", task.Stage)
	}
}

// reveal 收局: 撤掉场上的线索卡, 摆出答案。
func (q *Quiz) reveal(s *session, m *context.MessageManager, head, reason string) {
	task := q.task(s, len(q.rules.Stages), true)
	view := q.over(s, task, head, reason)
	if !q.submit(func() { q.send(s, task, view, "", true) }) {
		logger.Warnf("收局没排上出图队列, 改用文字")
		if err := q.notice(m, head); err != nil {
			logger.Warnf("收局回复失败: %v", err)
		}
	}
}

// send 渲染并发出这一张卡。
func (q *Quiz) send(s *session, task Task, view Caption, mention string, recallOld bool) error {
	q.mu.Lock()
	prev := s.message
	q.mu.Unlock()
	if recallOld && prev != "" {
		if err := q.sender.Recall(s.manager, prev); err != nil {
			logger.Warnf("撤回上一条线索失败(不影响这一局): %v", err)
		}
	}
	img, err := q.source.Draw(task)
	if err != nil {
		logger.Warnf("第 %d 层出图失败: %v", task.Stage, err)
		return nil
	}
	tplID, args := q.views.Caption(view)
	out := Outgoing{
		Image: img, TemplateID: tplID, Args: args, Keyboard: q.Keyboard(),
		NoFooter: true, Mention: mention,
	}
	id, err := q.sender.Post(s.manager, out)
	if err != nil {
		logger.Warnf("第 %d 层发送失败: %v", task.Stage, err)
		return err
	}
	q.mu.Lock()
	s.message = id
	q.mu.Unlock()
	return nil
}

// notice 是"一句话"的出口: 卡面仍由插件排, 本包只把话递过去。
func (q *Quiz) notice(m *context.MessageManager, text string) error {
	tplID, args := q.views.Caption(Caption{Kind: KindNotice, Reason: text})
	_, err := q.sender.Post(m, Outgoing{TemplateID: tplID, Args: args, Mention: m.UserId})
	return err
}

// over 造收局那张卡的输入: 带上用时与这一局揭过的提示。
func (q *Quiz) over(s *session, task Task, head, reason string) Caption {
	q.mu.Lock()
	used := append([]Hint(nil), s.used...)
	q.mu.Unlock()
	return Caption{
		Kind: KindOver, Task: task, Percent: 100, Head: head, Reason: reason,
		Elapsed: q.clock().Sub(s.started), Used: used,
	}
}

// task 按层号造一份输入; final 为真表示这是收局揭晓那张。
func (q *Quiz) task(s *session, stage int, final bool) Task {
	st := q.rules.Stages[clamp(stage-1, 0, len(q.rules.Stages)-1)]
	variant := st.Variant
	if variant == "" {
		variant = s.variant
	}
	level := st.Level
	if final {
		level = 1
	}
	return Task{
		Answer: s.answer, Stage: clamp(stage, 1, len(q.rules.Stages)), Stages: len(q.rules.Stages),
		Level: level, Variant: variant, Final: final, Seed: s.seed,
	}
}

// finish 收掉一局: 让节拍 goroutine 退出, 并把桌子让出来。
func (q *Quiz) finish(s *session) {
	s.once.Do(func() {
		q.mu.Lock()
		s.done = true
		delete(q.rounds, s.key)
		q.mu.Unlock()
		close(s.stop)
	})
}

func (q *Quiz) lookup(k string) *session {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.rounds[k]
	if s == nil || s.done {
		return nil
	}
	return s
}

func (q *Quiz) submit(fn func()) bool {
	if q.jobs == nil {
		fn()
		return true
	}
	return q.jobs.Submit(fn)
}

func wait(s *session, at time.Time) bool {
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.stop:
		return false
	}
}

// nextHint 取下一档里的一条提示; 没料的档直接跳过, 也不占这一次机会。
func nextHint(tiers [][]Hint, cursor *int) (Hint, bool) {
	for tier := *cursor; tier < len(tiers); tier++ {
		*cursor = tier + 1
		if pool := tiers[tier]; len(pool) > 0 {
			return pool[rand.IntN(len(pool))], true
		}
	}
	return Hint{}, false
}

func key(group, user string) string {
	if group != "" {
		return "g:" + group
	}
	return "u:" + user
}

func percent(level float64) int { return int(level*100 + 0.5) }

func round(d time.Duration) string { return d.Round(time.Second).String() }

func pick(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

const (
	seedPattern = 0x9E3779B97F4A7C15
	hintData    = "hint"
)
