package templates

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const inlineCommandFunc = "inline"

type inlineOperand struct {
	value   string
	literal bool
}

func isInlineCall(tag string) bool {
	_, ok := inlineCallBody(tag)
	return ok
}

func inlineArgNames(tag string) []string {
	label, command, err := parseInlineCall(tag)
	if err != nil {
		return nil
	}
	var names []string
	for _, op := range []inlineOperand{label, command} {
		if !op.literal {
			names = append(names, op.value)
		}
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
		if !isInlineCall(tag) {
			return match
		}
		rendered, err := renderInlineCommand(tag, flat)
		if err != nil {
			firstErr = err
			return match
		}
		return rendered
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func renderInlineCommand(tag string, flat map[string]string) (string, error) {
	labelOp, commandOp, err := parseInlineCall(tag)
	if err != nil {
		return "", err
	}
	label, err := resolveInlineOperand(labelOp, flat)
	if err != nil {
		return "", err
	}
	command, err := resolveInlineOperand(commandOp, flat)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(label) == "" {
		return "", fmt.Errorf("inline label must not be empty")
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("inline command must not be empty")
	}
	return fmt.Sprintf("[%s](mqqapi://aio/inlinecmd?command=%s&reply=false&enter=true)",
		escapeMarkdownLinkText(label), url.QueryEscape(command)), nil
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

func parseInlineCall(tag string) (inlineOperand, inlineOperand, error) {
	body, ok := inlineCallBody(tag)
	if !ok {
		return inlineOperand{}, inlineOperand{}, fmt.Errorf("not an inline call: %s", tag)
	}
	parts, err := parseInlineOperands(body)
	if err != nil {
		return inlineOperand{}, inlineOperand{}, err
	}
	if len(parts) != 2 {
		return inlineOperand{}, inlineOperand{}, fmt.Errorf("inline expects 2 args: label command")
	}
	return parts[0], parts[1], nil
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

func escapeMarkdownLinkText(input string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"[", "\\[",
		"]", "\\]",
		"\r", " ",
		"\n", " ",
	)
	return replacer.Replace(input)
}
