package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// BodyText, when set, turns a rich-text body (a Tiptap document) into
// plain text for the CSV and Markdown formats, which otherwise would show
// the raw document JSON. cmd wires it to its own bodyToText.
var BodyText func(doc map[string]any) string

// field and object keep the key order of the command's JSON struct, so
// CSV columns and YAML keys come out in a stable, meaningful order.
type field struct {
	key string
	val any
}
type object []field

// toOrdered round-trips v through JSON into ordered objects, []any,
// json.Number, string, bool and nil.
func toOrdered(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return decodeValue(dec)
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := object{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj = append(obj, field{key: kt.(string), val: val})
			}
			_, err := dec.Token()
			return obj, err
		case '[':
			list := []any{}
			for dec.More() {
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, val)
			}
			_, err := dec.Token()
			return list, err
		}
	}
	return tok, nil
}

// rows flattens data into a header and rows for CSV and Markdown:
//   - a list of objects -> one row per object, columns = all keys in order;
//   - an object with exactly one list field (e.g. {"notes": [...],
//     "has_more": ...}) -> that list, paging fields dropped;
//   - any other object -> "field" / "value" rows;
//   - a list of scalars -> one "value" column.
func rows(data any) ([]string, [][]string) {
	switch d := data.(type) {
	case object:
		if list, ok := soleList(d); ok {
			return rows(list)
		}
		out := make([][]string, 0, len(d))
		for _, f := range d {
			out = append(out, []string{f.key, cellText(f.key, f.val)})
		}
		return []string{"field", "value"}, out
	case []any:
		var header []string
		seen := map[string]bool{}
		for _, item := range d {
			obj, ok := item.(object)
			if !ok {
				header = nil
				break
			}
			for _, f := range obj {
				if !seen[f.key] {
					seen[f.key] = true
					header = append(header, f.key)
				}
			}
		}
		if header == nil {
			out := make([][]string, 0, len(d))
			for _, item := range d {
				out = append(out, []string{cellText("", item)})
			}
			return []string{"value"}, out
		}
		out := make([][]string, 0, len(d))
		for _, item := range d {
			obj := item.(object)
			row := make([]string, len(header))
			for i, h := range header {
				for _, f := range obj {
					if f.key == h {
						row[i] = cellText(h, f.val)
						break
					}
				}
			}
			out = append(out, row)
		}
		return header, out
	}
	return []string{"value"}, [][]string{{cellText("", data)}}
}

func soleList(obj object) ([]any, bool) {
	var found []any
	n := 0
	for _, f := range obj {
		if list, ok := f.val.([]any); ok {
			found = list
			n++
		}
	}
	return found, n == 1
}

// cellText renders one value for a CSV/Markdown cell: scalars as text,
// rich-text bodies as plain text, anything else as compact JSON.
func cellText(key string, v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	case []any:
		if allStrings(t) {
			parts := make([]string, len(t))
			for i, s := range t {
				parts[i] = s.(string)
			}
			return strings.Join(parts, ", ")
		}
	case object:
		if key == "body" && BodyText != nil && isDoc(t) {
			return BodyText(plainMap(t).(map[string]any))
		}
	}
	raw, _ := json.Marshal(plainMap(v))
	return string(raw)
}

func allStrings(list []any) bool {
	for _, v := range list {
		if _, ok := v.(string); !ok {
			return false
		}
	}
	return true
}

func isDoc(obj object) bool {
	for _, f := range obj {
		if f.key == "type" {
			return f.val == "doc"
		}
	}
	return false
}

// plainMap converts ordered values back to plain maps/slices.
func plainMap(v any) any {
	switch t := v.(type) {
	case object:
		m := make(map[string]any, len(t))
		for _, f := range t {
			m[f.key] = plainMap(f.val)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = plainMap(item)
		}
		return out
	}
	return v
}

func writeCSV(w io.Writer, data any) error {
	header, body := rows(data)
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	if err := cw.WriteAll(body); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func writeMarkdown(w io.Writer, data any) error {
	header, body := rows(data)
	esc := func(s string) string {
		s = strings.ReplaceAll(s, "|", `\|`)
		s = strings.ReplaceAll(s, "\r\n", "<br>")
		return strings.ReplaceAll(s, "\n", "<br>")
	}
	var b strings.Builder
	line := func(cells []string) {
		b.WriteString("|")
		for _, c := range cells {
			b.WriteString(" " + esc(c) + " |")
		}
		b.WriteString("\n")
	}
	line(header)
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = "---"
	}
	line(sep)
	for _, r := range body {
		line(r)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// YAML is emitted by hand (no dependency): JSON-quoted strings are valid
// YAML double-quoted scalars, so only safe words are left unquoted.
var plainScalar = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]*$`)

var yamlReserved = map[string]bool{
	"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true,
	"null": true, "y": true, "n": true, "~": true,
}

func yamlString(s string) string {
	if plainScalar.MatchString(s) && !yamlReserved[strings.ToLower(s)] {
		return s
	}
	q, _ := json.Marshal(s)
	return string(q)
}

func yamlLines(v any) []string {
	switch t := v.(type) {
	case nil:
		return []string{"null"}
	case string:
		return []string{yamlString(t)}
	case json.Number:
		return []string{t.String()}
	case bool:
		return []string{fmt.Sprint(t)}
	case object:
		if len(t) == 0 {
			return []string{"{}"}
		}
		var out []string
		for _, f := range t {
			child := yamlLines(f.val)
			key := yamlString(f.key)
			if isInline(f.val) {
				out = append(out, key+": "+child[0])
				continue
			}
			out = append(out, key+":")
			for _, c := range child {
				out = append(out, "  "+c)
			}
		}
		return out
	case []any:
		if len(t) == 0 {
			return []string{"[]"}
		}
		var out []string
		for _, item := range t {
			child := yamlLines(item)
			out = append(out, "- "+child[0])
			for _, c := range child[1:] {
				out = append(out, "  "+c)
			}
		}
		return out
	}
	return []string{yamlString(fmt.Sprint(v))}
}

func isInline(v any) bool {
	switch t := v.(type) {
	case object:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return true
}

func writeYAML(w io.Writer, v any) error {
	_, err := io.WriteString(w, strings.Join(yamlLines(v), "\n")+"\n")
	return err
}
