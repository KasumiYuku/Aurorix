package templates

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Markdown struct {
	Content string `json:"content"`
}

// 实现omitzero
func (m Markdown) IsZero() bool {
	return m.Content == ""
}

type MarkdownTemplate struct {
	Id       string
	Template string
	args     []string
}

// Args 模板参数，支持任意嵌套的 map/slice。
type Args map[string]any

var log = logx.New("templates")

var (
	regMu   sync.Mutex
	regMap  = make(map[string]map[string]*MarkdownTemplate)
	regSnap atomic.Value
)

func publish() {
	next := make(map[string]map[string]*MarkdownTemplate, len(regMap))
	for ns, m := range regMap {
		cp := make(map[string]*MarkdownTemplate, len(m))
		for id, t := range m {
			cp[id] = t
		}
		next[ns] = cp
	}
	regSnap.Store(next)
}

func register(ns, id, content string) {
	template, args := processTemplate(content)
	regMu.Lock()
	m := regMap[ns]
	if m == nil {
		m = make(map[string]*MarkdownTemplate)
		regMap[ns] = m
	}
	if _, dup := m[id]; dup {
		log.Warnf("模板 %v/%v 重复注册, 已覆盖", ns, id)
	}
	m[id] = &MarkdownTemplate{Id: id, Template: template, args: args}
	publish()
	regMu.Unlock()
}

// RegisterFS 把 FS 内 dir 子树下的 *.md 注册到命名空间。
func RegisterFS(ns string, fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		register(ns, strings.TrimSuffix(d.Name(), ".md"), string(content))
		return nil
	})
}

// NewMarkdownTemplate 注册单个模板到全局命名空间。
func NewMarkdownTemplate(Id string, Template string) {
	register("", Id, Template)
}

// IsMarkdownTemplateExit 任意命名空间是否存在该模板。
func IsMarkdownTemplateExit(Id string) bool {
	for _, m := range regSnap.Load().(map[string]map[string]*MarkdownTemplate) {
		if m[Id] != nil {
			return true
		}
	}
	return false
}

// ToMapString 把嵌套 Args 展开为扁平 map。
func ToMapString(h Args) (map[string]string, error) {
	result := make(map[string]string)
	var walk func(prefix string, v any) error
	walk = func(prefix string, v any) error {
		switch val := v.(type) {
		case string:
			result[prefix] = val
		case bool:
			result[prefix] = strconv.FormatBool(val)
		case int:
			result[prefix] = strconv.Itoa(val)
		case int64:
			result[prefix] = strconv.FormatInt(val, 10)
		case float64:
			result[prefix] = strconv.FormatFloat(val, 'f', -1, 64)
		case map[string]any:
			for k, sub := range val {
				key := prefix + "." + k
				if err := walk(key, sub); err != nil {
					return err
				}
			}
		case []any:
			for i, sub := range val {
				if err := walk(prefix+".#"+strconv.Itoa(i), sub); err != nil {
					return err
				}
			}
		case nil:
		default:
			return fmt.Errorf("key %s has unsupported type: %T", prefix, v)
		}
		return nil
	}
	for k, v := range h {
		if err := walk(k, v); err != nil {
			return nil, err
		}
	}
	return result, nil
}

var placeholderRe = regexp.MustCompile(`\{\{(.*?)\}\}`)

func processTemplate(input string) (string, []string) {
	var args []string
	seen := make(map[string]bool)
	inEach := false
	result := placeholderRe.ReplaceAllStringFunc(input, func(match string) string {
		trimmed := strings.TrimSpace(match[2 : len(match)-2])
		switch {
		case strings.HasPrefix(trimmed, "#each"):
			inEach = true
			return "{{" + trimmed + "}}"
		case trimmed == "/each":
			inEach = false
			return "{{/each}}"
		}
		if isInlineCall(trimmed) {
			if !inEach {
				for _, name := range inlineArgNames(trimmed) {
					if name != "" && !seen[name] {
						seen[name] = true
						args = append(args, name)
					}
				}
			}
			return "{{" + trimmed + "}}"
		}
		if trimmed != "" && !seen[trimmed] && !inEach {
			seen[trimmed] = true
			args = append(args, trimmed)
		}
		return "{{" + trimmed + "}}"
	})
	return result, args
}

var imageRe = regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)

// ProcessMarkdownImages 处理 markdown 图片引用并附带尺寸。
func ProcessMarkdownImages(input string) string {
	return imageRe.ReplaceAllStringFunc(input, func(match string) string {
		submatch := imageRe.FindStringSubmatch(match)
		alt, url := submatch[1], submatch[2]
		width, height, err := images.GetImageDimensions(url)
		if err != nil {
			return match
		}
		return fmt.Sprintf("![%s #%dpx #%dpx](%s)\n", alt, width, height, url)
	})
}

func processEach(template string, arg Args, flat map[string]string) (string, error) {
	if !strings.Contains(template, "{{#each") {
		return template, nil
	}
	var out strings.Builder
	rest := template
	for {
		start := strings.Index(rest, "{{#each")
		if start < 0 {
			out.WriteString(rest)
			break
		}
		headEnd := strings.Index(rest[start:], "}}")
		if headEnd < 0 {
			return "", fmt.Errorf("invalid each tag: %s", rest[start:])
		}
		head := rest[start : start+headEnd+2]
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(head, "}}"), "{{#each"))
		if key == "" {
			return "", fmt.Errorf("invalid each tag: missing key")
		}
		bodyStart := start + len(head)
		tailIdx := strings.Index(rest[bodyStart:], "{{/each}}")
		if tailIdx < 0 {
			return "", fmt.Errorf("each %s: missing {{/each}}", key)
		}
		body := rest[bodyStart : bodyStart+tailIdx]
		if strings.Contains(body, "{{#each") {
			return "", fmt.Errorf("each %s: nested each not supported", key)
		}
		arr, ok := arg[key].([]any)
		if !ok {
			return "", fmt.Errorf("each %s: arg must be []any, got %T", key, arg[key])
		}
		out.WriteString(rest[:start])
		for i, item := range arr {
			if _, ok := item.(map[string]any); !ok {
				return "", fmt.Errorf("each %s: item %d must be map[string]any, got %T", key, i, item)
			}
			seg := body
			prefix := key + ".#" + strconv.Itoa(i) + "."
			scope := make(map[string]string, len(flat))
			for fk, fv := range flat {
				scope[fk] = fv
			}
			for fk, fv := range flat {
				if strings.HasPrefix(fk, prefix) {
					local := fk[len(prefix):]
					scope[local] = fv
					seg = strings.ReplaceAll(seg, "{{"+local+"}}", fv)
				}
			}
			var err error
			seg, err = processInlineCommands(seg, scope)
			if err != nil {
				return "", err
			}
			out.WriteString(seg)
		}
		rest = rest[bodyStart+tailIdx+len("{{/each}}"):]
	}
	return out.String(), nil
}

// FillMarkdownTemplate 全局命名空间填充模板并校验是否仍有未填充项。
func FillMarkdownTemplate(Id string, arg Args) (string, error) {
	return fillFor("", Id, arg)
}

// FillFor 插件命名空间优先填充, 未命中回落全局。
func FillFor(ns, Id string, arg Args) (string, error) {
	return fillFor(ns, Id, arg)
}

func fillFor(ns, Id string, arg Args) (string, error) {
	snap := regSnap.Load().(map[string]map[string]*MarkdownTemplate)
	if ns != "" {
		if m := snap[ns]; m != nil {
			if t := m[Id]; t != nil {
				return fill(t, arg)
			}
		}
	}
	if m := snap[""]; m != nil {
		if t := m[Id]; t != nil {
			return fill(t, arg)
		}
	}
	return "", fmt.Errorf("Template %v not found", Id)
}

func fill(t *MarkdownTemplate, arg Args) (string, error) {
	flat, err := ToMapString(arg)
	if err != nil {
		return "", err
	}
	template := t.Template
	template, err = processEach(template, arg, flat)
	if err != nil {
		return "", err
	}
	template, err = processInlineCommands(template, flat)
	if err != nil {
		return "", err
	}
	for key, value := range flat {
		template = strings.ReplaceAll(template, "{{"+key+"}}", value)
	}
	_, after := processTemplate(template)
	if len(after) > 0 {
		return "", fmt.Errorf("Lost args: %s", strings.Join(after, ", "))
	}
	return template, nil
}

// GetMarkdownTemplateCount 全部命名空间的模板总数。
func GetMarkdownTemplateCount() uint {
	var n uint
	for _, m := range regSnap.Load().(map[string]map[string]*MarkdownTemplate) {
		n += uint(len(m))
	}
	return n
}
