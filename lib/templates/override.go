package templates

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 覆盖层目录布局: <root>/<命名空间>/<模板id>.<md|html>, 全局命名空间写作 _global。
const (
	GlobalNamespace      = "_global"
	DefaultOverrideDir   = "templates/override"
	DefaultReloadSeconds = 3
)

type Overrides struct {
	root     string
	interval time.Duration
	mu       sync.Mutex
	seen     map[baseKey]fileState
}

type fileState struct {
	size    int64
	modTime time.Time
}

func NewOverrides(root string, interval time.Duration) *Overrides {
	if interval <= 0 {
		interval = DefaultReloadSeconds * time.Second
	}
	return &Overrides{
		root:     strings.TrimSpace(root),
		interval: interval,
		seen:     make(map[baseKey]fileState),
	}
}

func (o *Overrides) Root() string { return o.root }

func (o *Overrides) Load() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.scan()
}

func (o *Overrides) Watch(ctx context.Context) {
	if o.root == "" {
		return
	}
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := o.Load(); err != nil {
				log.Warnf("模板覆盖层重扫失败: %v", err)
			}
		}
	}
}

func (o *Overrides) scan() error {
	if o.root == "" {
		return nil
	}
	if info, err := os.Stat(o.root); err != nil || !info.IsDir() {
		o.revertAll()
		return nil
	}

	next := make(map[baseKey]fileState)
	err := filepath.WalkDir(o.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		key, ok := overrideKey(o.root, path)
		if !ok {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		state := fileState{size: info.Size(), modTime: info.ModTime()}
		next[key] = state
		if prev, seen := o.seen[key]; seen && prev == state {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Warnf("模板覆盖层读取失败 %s: %v", path, readErr)
			return nil
		}
		put(key.kind, key.ns, key.id, string(content))
		return nil
	})

	for key := range o.seen {
		if _, ok := next[key]; ok {
			continue
		}
		o.revert(key)
	}
	o.seen = next
	return err
}

func (o *Overrides) revertAll() {
	for key := range o.seen {
		o.revert(key)
	}
	o.seen = make(map[baseKey]fileState)
}

func (o *Overrides) revert(key baseKey) {
	if content, ok := baseOf(key); ok {
		put(key.kind, key.ns, key.id, content)
		return
	}
	drop(key.kind, key.ns, key.id)
}

func overrideKey(root, path string) (baseKey, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return baseKey{}, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 2 {
		return baseKey{}, false
	}
	ns, name := parts[0], parts[1]
	if ns == GlobalNamespace {
		ns = ""
	}
	switch {
	case strings.HasSuffix(name, ".md"):
		return baseKey{ns: ns, kind: kindMarkdown, id: strings.TrimSuffix(name, ".md")}, true
	case strings.HasSuffix(name, ".html"):
		return baseKey{ns: ns, kind: kindHTML, id: strings.TrimSuffix(name, ".html")}, true
	}
	return baseKey{}, false
}
