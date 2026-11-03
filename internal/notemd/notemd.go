// Package notemd renders a note's Tiptap body as Markdown -- a mirror of
// the backend's app/services/note_export.py (tiptap_to_markdown), so a
// Zero-Knowledge note exported on this device reads exactly like a
// Default-tier note exported by the server. Keep the two in step.
package notemd

import (
	"fmt"
	"strings"
)

var headingPrefix = map[int]string{1: "# ", 2: "## ", 3: "### ", 4: "#### ", 5: "##### ", 6: "###### "}

// ToMarkdown renders body; unknown nodes and marks are skipped.
func ToMarkdown(body map[string]any) string {
	if len(body) == 0 {
		return ""
	}
	return strings.Join(renderBlock(body, 0), "\n\n")
}

func children(n map[string]any) []map[string]any {
	list, _ := n["content"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func attrs(n map[string]any) map[string]any {
	a, _ := n["attrs"].(map[string]any)
	return a
}

func attrString(n map[string]any, key string) string {
	switch v := attrs(n)[key].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func str(n map[string]any, key string) string {
	s, _ := n[key].(string)
	return s
}

func renderMarks(text string, marks []any) string {
	for _, m := range marks {
		mark, ok := m.(map[string]any)
		if !ok {
			continue
		}
		switch str(mark, "type") {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "_" + text + "_"
		case "strike":
			text = "~~" + text + "~~"
		case "code":
			text = "`" + text + "`"
		case "highlight":
			text = "==" + text + "=="
		case "link":
			text = "[" + text + "](" + attrString(mark, "href") + ")"
		}
	}
	return text
}

func renderInline(nodes []map[string]any) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch str(n, "type") {
		case "text":
			marks, _ := n["marks"].([]any)
			sb.WriteString(renderMarks(str(n, "text"), marks))
		case "hardBreak":
			sb.WriteString("  \n")
		case "mathInline":
			sb.WriteString("$" + attrString(n, "latex") + "$")
		case "noteLink":
			sb.WriteString("[[note:" + attrString(n, "id") + "]]")
		case "image":
			sb.WriteString("![" + attrString(n, "alt") + "](" + attrString(n, "src") + ")")
		}
	}
	return sb.String()
}

func renderList(n map[string]any, ordered bool, depth int) []string {
	var lines []string
	indent := strings.Repeat("  ", depth)
	for i, item := range children(n) {
		if str(item, "type") != "listItem" {
			continue
		}
		marker := "-"
		if ordered {
			marker = fmt.Sprintf("%d.", i+1)
		}
		itemLines := renderBlock(item, depth+1)
		if len(itemLines) == 0 {
			lines = append(lines, indent+marker)
			continue
		}
		lines = append(lines, indent+marker+" "+strings.TrimLeft(itemLines[0], " \t\n\r\v\f"))
		lines = append(lines, itemLines[1:]...)
	}
	return lines
}

func renderBlock(n map[string]any, depth int) []string {
	var lines []string
	for _, child := range children(n) {
		switch str(child, "type") {
		case "paragraph":
			lines = append(lines, renderInline(children(child)))
		case "heading":
			level := 1
			if l, ok := attrs(child)["level"].(float64); ok {
				level = int(l)
			}
			prefix, ok := headingPrefix[level]
			if !ok {
				prefix = "# "
			}
			lines = append(lines, prefix+renderInline(children(child)))
		case "blockquote":
			for _, inner := range renderBlock(child, depth) {
				if inner == "" {
					lines = append(lines, ">")
				} else {
					lines = append(lines, "> "+inner)
				}
			}
		case "codeBlock":
			var code strings.Builder
			for _, t := range children(child) {
				if str(t, "type") == "text" {
					code.WriteString(str(t, "text"))
				}
			}
			lines = append(lines, "```"+attrString(child, "language"))
			lines = append(lines, strings.Split(code.String(), "\n")...)
			lines = append(lines, "```")
		case "bulletList":
			lines = append(lines, renderList(child, false, depth)...)
		case "orderedList":
			lines = append(lines, renderList(child, true, depth)...)
		case "horizontalRule":
			lines = append(lines, "---")
		case "mathBlock":
			lines = append(lines, "$$"+attrString(child, "latex")+"$$")
		}
	}
	return lines
}
