package canvasrender

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func elements(t *testing.T, raw ...map[string]any) []Element {
	t.Helper()
	in := make([]any, len(raw))
	for i, r := range raw {
		in[i] = r
	}
	out := NormalizeElements(in)
	if len(out) != len(raw) {
		t.Fatalf("normalised %d of %d elements", len(out), len(raw))
	}
	return out
}

func rgba(img *image.RGBA, x, y int) color.RGBA {
	return img.RGBAAt(x, y)
}

func TestRenderFramesDrawingWithMargin(t *testing.T) {
	els := elements(t, map[string]any{
		"id": "a", "type": "shape", "x": 100.0, "y": 50.0, "w": 200.0, "h": 100.0,
		"data": map[string]any{"shapeKind": "rectangle", "strokeColor": "#ff0000", "fillColor": "#00ff00"},
	})
	img, err := Render(els, Options{Palette: LightPalette})
	if err != nil {
		t.Fatal(err)
	}
	// (200 + 2·16) × (100 + 2·16) units at 2 px per unit.
	if got := img.Bounds().Size(); got != image.Pt(464, 264) {
		t.Fatalf("size %v", got)
	}
	if c := rgba(img, 4, 4); c != (color.RGBA{250, 249, 247, 255}) {
		t.Errorf("margin %v, want background", c)
	}
	if c := rgba(img, 232, 132); c != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("centre %v, want fill", c)
	}
	// The stroke runs along the box edge, 16 units in.
	if c := rgba(img, 232, 32); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("edge %v, want stroke", c)
	}
}

func TestRenderCapsLongestSide(t *testing.T) {
	els := elements(t, map[string]any{
		"id": "a", "type": "shape", "x": 0.0, "y": 0.0, "w": 20000.0, "h": 10.0,
		"data": map[string]any{"shapeKind": "rectangle"},
	})
	img, err := Render(els, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if w := img.Bounds().Dx(); w > exportMaxSide {
		t.Fatalf("width %d over %d", w, exportMaxSide)
	}
}

func TestRenderEmpty(t *testing.T) {
	if _, err := Render(nil, Options{}); err != ErrEmpty {
		t.Fatalf("got %v", err)
	}
}

func TestRenderAllElementTypes(t *testing.T) {
	// A 1×1 red PNG.
	png := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="
	els := elements(t,
		map[string]any{"id": "t", "type": "text", "x": 0.0, "y": 0.0, "w": 120.0, "h": 40.0,
			"data": map[string]any{"text": "Hello wörld, wrapping text", "color": "#112233"}},
		map[string]any{"id": "m", "type": "text", "x": 0.0, "y": 60.0, "w": 120.0, "h": 20.0, "rotation": 30.0,
			"data": map[string]any{"text": "mono", "fontFamily": "mono"}},
		map[string]any{"id": "e", "type": "shape", "x": 150.0, "y": 0.0, "w": 80.0, "h": 40.0,
			"data": map[string]any{"shapeKind": "ellipse"}},
		map[string]any{"id": "l", "type": "shape", "x": 150.0, "y": 50.0, "w": 80.0, "h": 40.0,
			"data": map[string]any{"shapeKind": "line"}},
		map[string]any{"id": "r", "type": "arrow", "x": 0.0, "y": 100.0, "w": 100.0, "h": 50.0,
			"data": map[string]any{"x1": 0.0, "y1": 50.0, "x2": 100.0, "y2": 0.0}},
		map[string]any{"id": "k", "type": "ink", "x": 120.0, "y": 100.0, "w": 50.0, "h": 50.0,
			"data": map[string]any{"points": []any{
				map[string]any{"x": 0.0, "y": 0.0, "pressure": 0.5},
				map[string]any{"x": 25.0, "y": 40.0},
				map[string]any{"x": 50.0, "y": 10.0, "pressure": 0.1},
			}}},
		map[string]any{"id": "d", "type": "ink", "x": 180.0, "y": 100.0, "w": 1.0, "h": 1.0,
			"data": map[string]any{"points": []any{map[string]any{"x": 0.0, "y": 0.0}}}},
		map[string]any{"id": "i", "type": "image", "x": 0.0, "y": 160.0, "w": 40.0, "h": 20.0,
			"data": map[string]any{"dataUrl": png}},
		map[string]any{"id": "n", "type": "note_ref", "x": 60.0, "y": 160.0, "w": 180.0, "h": 56.0,
			"data": map[string]any{"note_id": "note1"}},
		map[string]any{"id": "q", "type": "task_ref", "x": 60.0, "y": 230.0, "w": 120.0, "h": 56.0,
			"data": map[string]any{"task_id": "task1"}},
	)
	refs := func(kind, id string) (string, string) {
		if kind == "note" {
			return "A note with a very long title that has to wrap onto more than two lines", ""
		}
		return "Gone", "Deleted"
	}
	img, err := Render(els, Options{Palette: LightPalette, RefText: refs})
	if err != nil {
		t.Fatal(err)
	}
	// The image is fitted into 40×20, centred: a 20×20 red square at x 10..30.
	x, y := (20+16)*2, (170+16)*2
	if c := rgba(img, x, y); c.R < 200 || c.G > 60 {
		t.Errorf("image pixel %v, want red", c)
	}
	if c := rgba(img, (2+16)*2, (170+16)*2); c != (color.RGBA{250, 249, 247, 255}) {
		t.Errorf("letterbox %v, want background", c)
	}
	for _, format := range []string{"png", "jpeg"} {
		var buf bytes.Buffer
		if err := Encode(&buf, img, format); err != nil || buf.Len() == 0 {
			t.Fatalf("%s: %v", format, err)
		}
	}
	if err := Encode(&bytes.Buffer{}, img, "gif"); err == nil {
		t.Fatal("gif accepted")
	}
}

func TestSVGPathIconFillsWithHoles(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	var p path
	addSVGPath(&p, xform{cos: 1, scale: 2}, noteIconPath, 0, 0, 1)
	p.fill(img, color.Black)
	// Page outline is filled, the page interior (hole) is not, and the
	// text lines inside the hole are.
	if a := img.RGBAAt(2*5, 2*12).A; a < 200 {
		t.Errorf("outline alpha %d", a)
	}
	if a := img.RGBAAt(2*12, 2*6).A; a > 40 {
		t.Errorf("hole alpha %d", a)
	}
	if a := img.RGBAAt(2*12, 2*17).A; a < 200 {
		t.Errorf("line alpha %d", a)
	}
}

func TestParseColor(t *testing.T) {
	if _, err := ParseColor("#12ab34"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseColor("red"); err == nil {
		t.Fatal("accepted red")
	}
}
