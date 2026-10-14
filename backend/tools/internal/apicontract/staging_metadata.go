package apicontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// normalizeStagingMetadata 只修改 goctl 的临时 staging 副本。它让尚未完成
// 七字段迁移的历史 API 能通过固定 fork 的全图 Validate，同时不改真实源文件，
// 也不把临时 metadata 当成授权或文档事实。
func normalizeStagingMetadata(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".api" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		normalized, changed, err := normalizeStagingMetadataSource(string(data))
		if err != nil {
			return fmt.Errorf("normalize %s: %w", path, err)
		}
		if !changed {
			return nil
		}
		return os.WriteFile(path, []byte(normalized), 0o644)
	})
}

func normalizeStagingMetadataSource(source string) (string, bool, error) {
	lines := strings.SplitAfter(source, "\n")
	out := make([]string, 0, len(lines))
	changed := false

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "@doc") {
			start := i
			i++
			if strings.HasPrefix(trimmed, "@doc(") || strings.HasPrefix(trimmed, "@doc (") {
				depth := annotationParenDepth(lines[start])
				for depth > 0 && i < len(lines) {
					depth += annotationParenDepth(lines[i])
					i++
				}
				if depth != 0 {
					return "", false, fmt.Errorf("unterminated @doc block")
				}
			}
			doc := append([]string(nil), lines[start:i]...)
			var gap []string
			for i < len(lines) {
				next := strings.TrimSpace(lines[i])
				if next != "" && !strings.HasPrefix(next, "//") {
					break
				}
				gap = append(gap, lines[i])
				i++
			}
			if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "@handler ") {
				handler := stagingHandlerName(lines[i])
				normalized, docChanged := normalizeDocLines(doc, handler)
				out = append(out, normalized...)
				out = append(out, gap...)
				out = append(out, lines[i])
				i++
				changed = changed || docChanged
				continue
			}
			out = append(out, doc...)
			out = append(out, gap...)
			continue
		}
		if strings.HasPrefix(trimmed, "@handler ") {
			handler := stagingHandlerName(lines[i])
			indent := leadingWhitespace(lines[i])
			out = append(out, completeDoc(indent, handler, handler)...)
			out = append(out, lines[i])
			i++
			changed = true
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return strings.Join(out, ""), changed, nil
}

func normalizeDocLines(lines []string, handler string) ([]string, bool) {
	if len(lines) == 1 {
		trimmed := strings.TrimSpace(lines[0])
		if strings.HasPrefix(trimmed, "@doc ") && !strings.Contains(trimmed, "(") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "@doc"))
			summary, err := strconv.Unquote(raw)
			if err != nil || summary == "" {
				summary = handler
			}
			return completeDoc(leadingWhitespace(lines[0]), summary, handler), true
		}
	}

	keys := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if at := strings.IndexByte(trimmed, ':'); at > 0 {
			keys[strings.TrimSpace(trimmed[:at])] = true
		}
	}
	missing := missingMetadata(keys)
	if len(missing) == 0 {
		return lines, false
	}
	closeAt := len(lines) - 1
	for closeAt >= 0 && strings.TrimSpace(lines[closeAt]) == "" {
		closeAt--
	}
	if closeAt < 0 || strings.TrimSpace(lines[closeAt]) != ")" {
		return completeDoc(leadingWhitespace(lines[0]), handler, handler), true
	}
	indent := leadingWhitespace(lines[closeAt]) + "    "
	inserted := make([]string, 0, len(lines)+len(missing))
	inserted = append(inserted, lines[:closeAt]...)
	for _, key := range missing {
		inserted = append(inserted, indent+key+": "+strconv.Quote(defaultMetadataValue(key, handler))+"\n")
	}
	inserted = append(inserted, lines[closeAt:]...)
	return inserted, true
}

func completeDoc(indent, summary, handler string) []string {
	values := map[string]string{
		"summary":       summary,
		"description":   "仅用于兼容固定生成器的历史接口元数据：" + handler,
		"authType":      "admin",
		"resource":      "legacy-api",
		"action":        "view",
		"group":         "legacy",
		"recordLogMode": "1",
	}
	lines := []string{indent + "@doc(\n"}
	for _, key := range RequiredMetadataFields {
		lines = append(lines, indent+"    "+key+": "+strconv.Quote(values[key])+"\n")
	}
	return append(lines, indent+")\n")
}

func missingMetadata(keys map[string]bool) []string {
	var missing []string
	for _, key := range RequiredMetadataFields {
		if !keys[key] {
			missing = append(missing, key)
		}
	}
	return missing
}

func defaultMetadataValue(key, handler string) string {
	switch key {
	case "summary":
		return handler
	case "description":
		return "仅用于兼容固定生成器的历史接口元数据：" + handler
	case "authType":
		return "admin"
	case "resource":
		return "legacy-api"
	case "action":
		return "view"
	case "group":
		return "legacy"
	case "recordLogMode":
		return "1"
	default:
		return ""
	}
}

func stagingHandlerName(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) >= 2 {
		return fields[1]
	}
	return "legacyRoute"
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func annotationParenDepth(line string) int {
	depth := 0
	inQuote := false
	escaped := false
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return depth
}
