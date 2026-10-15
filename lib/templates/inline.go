package templates

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	inlineCommandFunc = "inline"
	inlineTextLimit   = 100
)

type inlineOperand struct {
	value   string
	literal bool
}

func isInlineCall(tag string) bool {
	_, ok := inlineCallBody(tag)
	return ok
}

func inlineArgNames(tag string) []string {
	call, err := parseInlineCall(tag)
	if err != nil {
		return nil
	}
	var names []string
	for _, op := range []inlineOperand{call.label, call.command} {
		if !op.literal {
			names = append(names, op.value)
		}
	}
	if call.hasReference && !call.reference.literal {
		names = append(names, call.reference.value)
	}
	return names
}

func processInlineCommands(template string, flat map[string]string) (string, error) {
	var firstErr error
	out := placeholderRe.ReplaceAllStringFunc(template, func(match string) string {
		if firstErr != nil {
			return match
		}
		tag := strings.TrimSpace(match[2 : len(match)-2])
		// 模板里可用的函数都在这里分发: 加一个只需加一条分支,
		// 而不必在填充流程里再加一个步骤。
		switch {
		case isInlineCall(tag):
			rendered, err := renderInlineCommand(tag, flat)
			if err != nil {
				firstErr = err
				return match
			}
			return rendered
		case isQQCardCall(tag):
			rendered, err := renderQQCard(tag, flat)
			if err != nil {
				firstErr = err
				return match
			}
			return rendered
		}
		return match
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func renderInlineCommand(tag string, flat map[string]string) (string, error) {
	call, err := parseInlineCall(tag)
	if err != nil {
		return "", err
	}
	label, err := resolveInlineOperand(call.label, flat)
	if err != nil {
		return "", err
	}
	command, err := resolveInlineOperand(call.command, flat)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(label) == "" {
		return "", fmt.Errorf("inline label must not be empty")
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("inline command must not be empty")
	}
	checkInlineLength("text", command)
	checkInlineLength("show", label)
	reference := false
	if call.hasReference {
		value, err := resolveInlineOperand(call.reference, flat)
		if err != nil {
			return "", err
		}
		reference = inlineTruthy(value)
	}
	return buildInlineCommand(command, label, reference), nil
}

// buildInlineCommand 拼官方 markdown 的「参数指令」标签: 点击把 text 填进输入框, 用户补完再发。
func buildInlineCommand(command, label string, reference bool) string {
	out := `<qqbot-cmd-input text="` + encodeValue(command) + `"`
	if label != command {
		out += ` show="` + encodeValue(label) + `"`
	}
	if reference {
		out += ` reference="true"`
	}
	return out + " />"
}

func inlineTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

func checkInlineLength(field, value string) {
	if n := utf8.RuneCountInString(value); n > inlineTextLimit {
		log.Warnf("inline 的 %s 有 %d 个字符, 超过官方上限 %d, 客户端可能不认这条点击指令", field, n, inlineTextLimit)
	}
}

func resolveInlineOperand(op inlineOperand, flat map[string]string) (string, error) {
	if op.literal {
		return op.value, nil
	}
	value, ok := flat[op.value]
	if !ok {
		return "", fmt.Errorf("inline arg %s not found", op.value)
	}
	return value, nil
}

type inlineCall struct {
	label        inlineOperand
	command      inlineOperand
	reference    inlineOperand
	hasReference bool
}

func parseInlineCall(tag string) (inlineCall, error) {
	body, ok := inlineCallBody(tag)
	if !ok {
		return inlineCall{}, fmt.Errorf("not an inline call: %s", tag)
	}
	parts, err := parseInlineOperands(body)
	if err != nil {
		return inlineCall{}, err
	}
	if len(parts) < 2 || len(parts) > 3 {
		return inlineCall{}, fmt.Errorf("inline expects 2 or 3 args: label command [reference]")
	}
	call := inlineCall{label: parts[0], command: parts[1]}
	if len(parts) == 3 {
		call.reference = parts[2]
		call.hasReference = true
	}
	return call, nil
}

func inlineCallBody(tag string) (string, bool) {
	if tag == inlineCommandFunc {
		return "", true
	}
	if !strings.HasPrefix(tag, inlineCommandFunc) {
		return "", false
	}
	rest := tag[len(inlineCommandFunc):]
	r, _ := utf8.DecodeRuneInString(rest)
	if !unicode.IsSpace(r) {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func parseInlineOperands(input string) ([]inlineOperand, error) {
	var parts []inlineOperand
	for i := 0; i < len(input); {
		for i < len(input) {
			r, size := utf8.DecodeRuneInString(input[i:])
			if !unicode.IsSpace(r) {
				break
			}
			i += size
		}
		if i >= len(input) {
			break
		}
		r, size := utf8.DecodeRuneInString(input[i:])
		if r == '"' || r == '\'' {
			value, next, err := parseQuotedInlineOperand(input, i, r, size)
			if err != nil {
				return nil, err
			}
			parts = append(parts, inlineOperand{value: value, literal: true})
			i = next
			continue
		}
		start := i
		for i < len(input) {
			r, size := utf8.DecodeRuneInString(input[i:])
			if unicode.IsSpace(r) {
				break
			}
			i += size
		}
		parts = append(parts, inlineOperand{value: input[start:i]})
	}
	return parts, nil
}

func parseQuotedInlineOperand(input string, start int, quote rune, quoteSize int) (string, int, error) {
	var out strings.Builder
	i := start + quoteSize
	for i < len(input) {
		r, size := utf8.DecodeRuneInString(input[i:])
		switch r {
		case '\\':
			i += size
			if i >= len(input) {
				return "", 0, fmt.Errorf("inline quoted arg has dangling escape")
			}
			next, nextSize := utf8.DecodeRuneInString(input[i:])
			out.WriteRune(next)
			i += nextSize
		case quote:
			return out.String(), i + size, nil
		default:
			out.WriteRune(r)
			i += size
		}
	}
	return "", 0, fmt.Errorf("inline quoted arg is not closed")
}
