package admin

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/schedule"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	topicOverview = "overview.snapshot"
	topicJobs     = "jobs.list"
	topicLog      = "log.entry"

	replayLimit = 512
)

type liveBus struct {
	deps Deps

	mu     sync.Mutex
	subs   map[*liveSub]struct{}
	gen    *liveGen
	seq    int64
	replay []replayFrame
}

type liveGen struct {
	ticker *time.Ticker
	stop   chan struct{}
}

type liveSub struct {
	ch chan []byte
}

type replayFrame struct {
	seq int64
	raw []byte
}

func newLiveBus(deps Deps) *liveBus {
	return &liveBus{deps: deps, subs: make(map[*liveSub]struct{})}
}

func encodeFrame(seq int64, topic string, payload []byte) []byte {
	envelope, err := json.Marshal(struct {
		Topic string          `json:"topic"`
		Seq   int64           `json:"seq"`
		At    int64           `json:"at"`
		Data  json.RawMessage `json:"data"`
	}{Topic: topic, Seq: seq, At: time.Now().UnixMilli(), Data: payload})
	if err != nil {
		return nil
	}
	frame := make([]byte, 0, len(envelope)+24)
	frame = append(frame, "id: "...)
	frame = strconv.AppendInt(frame, seq, 10)
	frame = append(frame, "\ndata: "...)
	frame = append(frame, envelope...)
	frame = append(frame, '\n', '\n')
	return frame
}

func (b *liveBus) encodeLocked(topic string, data any) []byte {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	b.seq++
	raw := encodeFrame(b.seq, topic, payload)
	b.replay = append(b.replay, replayFrame{seq: b.seq, raw: raw})
	if len(b.replay) > replayLimit {
		copy(b.replay, b.replay[len(b.replay)-replayLimit:])
		b.replay = b.replay[:replayLimit]
	}
	return raw
}

func (b *liveBus) publish(topic string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	raw := b.encodeLocked(topic, data)
	if raw == nil {
		return
	}
	for s := range b.subs {
		send(s, raw)
	}
}

func (b *liveBus) subscribe(lastEventID int64) *liveSub {
	b.mu.Lock()
	sub := &liveSub{ch: make(chan []byte, 16)}
	b.subs[sub] = struct{}{}
	needStart := b.gen == nil
	replayed := b.replayLocked(lastEventID)
	if replayed == nil {
		replayed = []replayFrame{
			{raw: b.encodeLocked(topicOverview, b.overviewView())},
			{raw: b.encodeLocked(topicJobs, schedule.Jobs())},
		}
	}
	b.mu.Unlock()

	if needStart {
		b.start()
	}
	for _, frame := range replayed {
		if frame.raw != nil {
			send(sub, frame.raw)
		}
	}
	return sub
}

func (b *liveBus) replayLocked(lastEventID int64) []replayFrame {
	if lastEventID <= 0 || len(b.replay) == 0 || lastEventID < b.replay[0].seq {
		return nil
	}
	out := make([]replayFrame, 0, len(b.replay))
	for _, frame := range b.replay {
		if frame.seq > lastEventID {
			out = append(out, frame)
		}
	}
	return out
}

func (b *liveBus) unsubscribe(sub *liveSub) {
	b.mu.Lock()
	_, ok := b.subs[sub]
	if ok {
		delete(b.subs, sub)
		close(sub.ch)
	}
	needStop := ok && len(b.subs) == 0 && b.gen != nil
	gen := b.gen
	if needStop {
		b.gen = nil
	}
	b.mu.Unlock()

	if needStop {
		close(gen.stop)
		gen.ticker.Stop()
		schedule.SetChangeHook(nil)
	}
}

func (b *liveBus) start() {
	b.mu.Lock()
	if b.gen != nil {
		b.mu.Unlock()
		return
	}
	gen := &liveGen{ticker: time.NewTicker(time.Second), stop: make(chan struct{})}
	b.gen = gen
	schedule.SetChangeHook(func() { b.bumpJobs() })
	b.mu.Unlock()

	go b.tickLoop(gen)
	go b.logLoop(gen)
}

func (b *liveBus) tickLoop(gen *liveGen) {
	for {
		select {
		case <-gen.stop:
			return
		case <-gen.ticker.C:
			b.publish(topicOverview, b.overviewView())
		}
	}
}

func (b *liveBus) logLoop(gen *liveGen) {
	ch, cancel := logx.Subscribe()
	defer cancel()
	for {
		select {
		case <-gen.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			b.publish(topicLog, e)
		}
	}
}

func (b *liveBus) bumpJobs() {
	b.mu.Lock()
	idle := len(b.subs) == 0
	b.mu.Unlock()
	if idle {
		return
	}
	b.publish(topicJobs, schedule.Jobs())
}

func send(s *liveSub, frame []byte) {
	select {
	case s.ch <- frame:
	default:
		for {
			select {
			case <-s.ch:
			default:
				goto drainDone
			}
		}
	drainDone:
		select {
		case s.ch <- frame:
		default:
		}
	}
}

func (b *liveBus) overviewView() H {
	view := overviewSnapshot(b.deps)
	view["now"] = time.Now().UnixMilli()
	return view
}

func (b *liveBus) handleStream(w http.ResponseWriter, r *http.Request) {
	lastEventID, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if lastEventID == 0 {
		lastEventID, _ = strconv.ParseInt(r.URL.Query().Get("last_event_id"), 10, 64)
	}
	sub := b.subscribe(lastEventID)
	defer b.unsubscribe(sub)

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame := <-sub.ch:
			if _, err := w.Write(frame); err != nil {
				return
			}
			rc.Flush()
		}
	}
}
