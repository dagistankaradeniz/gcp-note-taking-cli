// Package canvasrender draws a Quillink canvas (the shared Canvas v1
// element format) to a PNG or JPEG, for `quillink canvas export`. It is the
// CLI's mirror of the web renderer (frontend src/components/canvas/scene:
// contract.ts, geometry.ts, render.tsx) and the native ones (Android
// CanvasRenderer.kt, iOS CanvasElementsRenderer.swift); the contract rules
// are checked against the same fixtures (testdata/canvas_contract_fixtures.json,
// an identical copy of the other clients').
package canvasrender

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf16"
)

// Limits and defaults, as in contract.ts.
const (
	MaxElements          = 5_000
	maxCoordinate        = 1_000_000
	maxSize              = 100_000
	maxRelative          = 100_000
	maxInkPoints         = 5_000
	maxTextLength        = 10_000
	maxImageDataURLLen   = 2_800_000
	minStrokeWidth       = 0.5
	maxStrokeWidth       = 64
	minFontSize          = 6
	maxFontSize          = 400
	defaultStrokeWidth   = 3
	defaultFontSize      = 17
	defaultColor         = "#000000"
	textLineHeight       = 1.2
	inkMinPressure       = 0.3
	arrowHeadAngle       = math.Pi / 7
	contractEndpointKeys = "x1 y1 x2 y2"
)

var (
	elementTypes = map[string]bool{
		"text": true, "image": true, "shape": true, "arrow": true, "ink": true, "note_ref": true, "task_ref": true,
	}
	shapeKinds     = map[string]bool{"rectangle": true, "ellipse": true, "line": true}
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	colorPattern   = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	base64Pattern  = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
	imageMimeTypes = []string{"image/png", "image/jpeg", "image/webp"}
)

// Element is one canvas element in canonical form.
type Element = map[string]any

// NormalizeElements drops invalid elements, keeps the first of each id and
// caps the list -- normalizeElements in contract.ts. Elements are untrusted
// (collaborators, shares, agents and API clients write them).
func NormalizeElements(raw []any) []Element {
	seen := map[string]bool{}
	var out []Element
	for _, v := range raw {
		if len(out) >= MaxElements {
			break
		}
		el := NormalizeElement(v)
		if el == nil || seen[el["id"].(string)] {
			continue
		}
		seen[el["id"].(string)] = true
		out = append(out, el)
	}
	return out
}

// NormalizeElement returns one element in canonical form, or nil when it
// must be dropped.
func NormalizeElement(raw any) Element {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	id, ok := obj["id"].(string)
	if !ok || !idPattern.MatchString(id) {
		return nil
	}
	typ, ok := obj["type"].(string)
	if !ok || !elementTypes[typ] {
		return nil
	}
	input, _ := obj["data"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}
	data := normalizedData(typ, input)
	if data == nil {
		return nil
	}
	return Element{
		"id":       id,
		"type":     typ,
		"x":        clamp(numOr(obj["x"], 0), maxCoordinate),
		"y":        clamp(numOr(obj["y"], 0), maxCoordinate),
		"w":        size(obj["w"]),
		"h":        size(obj["h"]),
		"rotation": normalizedRotation(numOr(obj["rotation"], 0)),
		"style":    map[string]any{},
		"data":     data,
	}
}

// normalizedRotation is degrees in [0, 360).
func normalizedRotation(deg float64) float64 {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return 0
	}
	r := math.Mod(deg, 360)
	if r < 0 {
		r += 360
	}
	if r >= 360 || r == 0 {
		return 0
	}
	return r
}

func imageMimeType(dataURL string) string {
	if len(dataURL) > maxImageDataURLLen {
		return ""
	}
	for _, mime := range imageMimeTypes {
		prefix := "data:" + mime + ";base64,"
		if !strings.HasPrefix(dataURL, prefix) {
			continue
		}
		if base64Pattern.MatchString(dataURL[len(prefix):]) {
			return mime
		}
		return ""
	}
	return ""
}

func normalizedData(typ string, in map[string]any) map[string]any {
	out := map[string]any{}
	switch typ {
	case "shape":
		kind, _ := in["shapeKind"].(string)
		if !shapeKinds[kind] {
			kind = "rectangle"
		}
		out["shapeKind"] = kind
		putStroke(in, out)
		if c, ok := in["fillColor"].(string); ok && colorPattern.MatchString(c) {
			out["fillColor"] = c
		}
		if kind == "line" {
			putEndpoints(in, out)
		}
	case "arrow":
		putStroke(in, out)
		putEndpoints(in, out)
	case "ink":
		putStroke(in, out)
		points := []any{}
		list, _ := in["points"].([]any)
		for _, item := range list {
			if len(points) >= maxInkPoints {
				break
			}
			p, ok := item.(map[string]any)
			if !ok {
				continue
			}
			x, okX := num(p["x"])
			y, okY := num(p["y"])
			if !okX || !okY {
				continue
			}
			pressure := math.Min(math.Max(numOr(p["pressure"], 1), 0), 1)
			points = append(points, map[string]any{"x": clamp(x, maxRelative), "y": clamp(y, maxRelative), "pressure": pressure})
		}
		out["points"] = points
	case "text":
		text, _ := in["text"].(string)
		out["text"] = truncatedUTF16(text, maxTextLength)
		out["fontSize"] = clampRange(numOr(in["fontSize"], defaultFontSize), minFontSize, maxFontSize)
		out["color"] = colorOr(in["color"])
		if f, _ := in["fontFamily"].(string); f == "serif" || f == "mono" {
			out["fontFamily"] = f
		}
	case "image":
		dataURL, ok := in["dataUrl"].(string)
		if !ok {
			return nil
		}
		mime := imageMimeType(dataURL)
		if mime == "" {
			return nil
		}
		out["dataUrl"] = dataURL
		out["mimeType"] = mime
	case "note_ref", "task_ref":
		key := "note_id"
		if typ == "task_ref" {
			key = "task_id"
		}
		ref, ok := in[key].(string)
		if !ok || !idPattern.MatchString(ref) {
			return nil
		}
		out[key] = ref
		if in["show_content"] == true {
			out["show_content"] = true
		}
	}
	return out
}

func putStroke(in, out map[string]any) {
	out["strokeColor"] = colorOr(in["strokeColor"])
	out["strokeWidth"] = clampRange(numOr(in["strokeWidth"], defaultStrokeWidth), minStrokeWidth, maxStrokeWidth)
}

// putEndpoints copies all four endpoints or none.
func putEndpoints(in, out map[string]any) {
	keys := strings.Fields(contractEndpointKeys)
	values := make([]float64, len(keys))
	for i, k := range keys {
		v, ok := num(in[k])
		if !ok {
			return
		}
		values[i] = v
	}
	for i, k := range keys {
		out[k] = clamp(values[i], maxRelative)
	}
}

// num accepts real, finite JSON numbers only -- "10" counts as missing.
func num(v any) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func numOr(v any, def float64) float64 {
	if f, ok := num(v); ok {
		return f
	}
	return def
}

func colorOr(v any) string {
	if s, ok := v.(string); ok && colorPattern.MatchString(s) {
		return s
	}
	return defaultColor
}

func clamp(v, limit float64) float64 { return math.Min(math.Max(v, -limit), limit) }

func clampRange(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

func size(v any) float64 {
	f, ok := num(v)
	if !ok {
		f = 1
	}
	return clampRange(f, 1, maxSize)
}

// truncatedUTF16 keeps at most limit UTF-16 code units (JS string length),
// never splitting a surrogate pair.
func truncatedUTF16(s string, limit int) string {
	units := utf16.Encode([]rune(s))
	if len(units) <= limit {
		return s
	}
	cut := limit
	if u := units[limit-1]; u >= 0xd800 && u <= 0xdbff {
		cut--
	}
	return string(utf16.Decode(units[:cut]))
}
