package canvasrender

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/webp" // decodes embedded WebP images
)

// Export framing, as in the web's exportImage.ts.
const (
	exportMaxSide    = 8192
	exportPixelRatio = 2
	exportMargin     = 16
)

// Ref card layout, identical to render.tsx / iOS RefCard / Android drawRefCard.
const (
	cardRadius    = 8
	cardPadding   = 10
	cardIcon      = 15
	cardGap       = 8
	cardTitleSize = 15
)

// MUI DescriptionOutlined / Checklist, 24-unit paths (render.tsx).
const (
	noteIconPath = "M8 16h8v2H8zm0-4h8v2H8zm6-10H6c-1.1 0-2 .9-2 2v16c0 1.1.89 2 1.99 2H18c1.1 0 2-.9 2-2V8zm4 18H6V4h7v5h5z"
	taskIconPath = "M22 7h-9v2h9zm0 8h-9v2h9zM5.54 11 2 7.46l1.41-1.41 2.12 2.12 4.24-4.24 1.41 1.41zm0 8L2 15.46l1.41-1.41 2.12 2.12 4.24-4.24 1.41 1.41z"
)

// Palette is the scene's colours (the web's light theme by default).
type Palette struct {
	Background color.Color // nil = transparent
	Card       color.Color
	Foreground color.Color
	Muted      color.Color
}

// LightPalette and DarkPalette match the web's --color-background/card/
// foreground/muted-foreground tokens.
var (
	LightPalette = Palette{
		Background: color.RGBA{250, 249, 247, 255},
		Card:       color.RGBA{255, 255, 255, 255},
		Foreground: color.RGBA{10, 10, 10, 255},
		Muted:      color.RGBA{115, 115, 115, 255},
	}
	DarkPalette = Palette{
		Background: color.RGBA{24, 23, 21, 255},
		Card:       color.RGBA{30, 29, 27, 255},
		Foreground: color.RGBA{197, 197, 197, 255},
		Muted:      color.RGBA{171, 171, 171, 255},
	}
)

// RefText resolves a note/task card's title and status ("Deleted", "In
// trash", or ""). kind is "note" or "task".
type RefText func(kind, id string) (title, status string)

// Options control one render.
type Options struct {
	// Scale is output pixels per canvas unit (default 2, the web export's);
	// lowered automatically so the longest side stays within 8192 px.
	Scale   float64
	Palette Palette
	RefText RefText
}

// ErrEmpty is returned for a canvas with nothing to draw.
var ErrEmpty = errors.New("this canvas is empty")

// Render draws elements (already normalised) to an image framed around
// the drawing with a small margin, like the web's PNG export.
func Render(elements []Element, opt Options) (*image.RGBA, error) {
	if len(elements) == 0 {
		return nil, ErrEmpty
	}
	fs, err := loadFonts()
	if err != nil {
		return nil, fmt.Errorf("load fonts: %w", err)
	}
	minX, minY, maxX, maxY := contentBounds(elements)
	x0, y0 := minX-exportMargin, minY-exportMargin
	w, h := maxX-minX+2*exportMargin, maxY-minY+2*exportMargin
	scale := opt.Scale
	if scale <= 0 {
		scale = exportPixelRatio
	}
	scale = math.Min(scale, exportMaxSide/math.Max(w, h))
	img := image.NewRGBA(image.Rect(0, 0, max(1, int(math.Ceil(w*scale))), max(1, int(math.Ceil(h*scale)))))
	if opt.Palette.Background != nil {
		xdraw.Draw(img, img.Bounds(), image.NewUniform(opt.Palette.Background), image.Point{}, xdraw.Src)
	}
	r := renderer{dst: img, fonts: fs, buf: &sfnt.Buffer{}, opt: opt, ox: x0, oy: y0, scale: scale}
	for _, el := range elements {
		r.element(el)
	}
	return img, nil
}

// Encode writes img as PNG or JPEG ("png", "jpeg"/"jpg").
func Encode(w io.Writer, img image.Image, format string) error {
	switch strings.ToLower(format) {
	case "png":
		return png.Encode(w, img)
	case "jpeg", "jpg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
	}
	return fmt.Errorf("unknown image format %q (use png or jpeg)", format)
}

type renderer struct {
	dst    *image.RGBA
	fonts  fontSet
	buf    *sfnt.Buffer
	opt    Options
	ox, oy float64
	scale  float64
}

func (r *renderer) xf(el Element) xform {
	x, y, w, h := f(el, "x"), f(el, "y"), f(el, "w"), f(el, "h")
	rad := f(el, "rotation") * math.Pi / 180
	return xform{cx: x + w/2, cy: y + h/2, cos: math.Cos(rad), sin: math.Sin(rad), ox: r.ox, oy: r.oy, scale: r.scale}
}

func (r *renderer) element(el Element) {
	data := el["data"].(map[string]any)
	t := r.xf(el)
	switch el["type"] {
	case "shape", "arrow", "ink":
		r.stroked(el, data, t)
	case "text":
		r.text(el, data, t)
	case "image":
		r.image(el, data, t)
	case "note_ref":
		r.refCard(el, "note", data["note_id"].(string), t)
	case "task_ref":
		r.refCard(el, "task", data["task_id"].(string), t)
	}
}

// stroked draws shapes, arrows and ink with round caps and joins.
func (r *renderer) stroked(el Element, data map[string]any, t xform) {
	stroke := hexColor(data["strokeColor"].(string))
	width := data["strokeWidth"].(float64)
	x, y, w, h := f(el, "x"), f(el, "y"), f(el, "w"), f(el, "h")
	var p path

	if el["type"] == "ink" {
		points, _ := data["points"].([]any)
		pts := make([]struct{ x, y, pr float64 }, 0, len(points))
		for _, item := range points {
			m := item.(map[string]any)
			pts = append(pts, struct{ x, y, pr float64 }{x + m["x"].(float64), y + m["y"].(float64), m["pressure"].(float64)})
		}
		if len(pts) == 1 {
			p.addPositive(circlePoly(t, point{pts[0].x, pts[0].y}, width*math.Max(pts[0].pr, inkMinPressure)/2))
		}
		for i := 0; i+1 < len(pts); i++ {
			a, b := pts[i], pts[i+1]
			sw := width * math.Max((a.pr+b.pr)/2, inkMinPressure)
			strokeSegment(&p, t, point{a.x, a.y}, point{b.x, b.y}, sw)
		}
		p.fill(r.dst, stroke)
		return
	}

	kind := "arrow"
	if el["type"] == "shape" {
		kind = data["shapeKind"].(string)
	}
	switch kind {
	case "line", "arrow":
		start, end := endpoints(el, data)
		strokeSegment(&p, t, start, end, width)
		if kind == "arrow" {
			b1, b2 := arrowHead(start, end, width)
			strokeSegment(&p, t, end, b1, width)
			strokeSegment(&p, t, end, b2, width)
		}
		p.fill(r.dst, stroke)
		return
	}

	var outline []point
	if kind == "ellipse" {
		outline = ellipsePoints(x+w/2, y+h/2, w/2, h/2, r.scale)
	} else {
		outline = []point{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
	}
	if fill, ok := data["fillColor"].(string); ok {
		var fp path
		poly := make([]point, len(outline))
		for i, q := range outline {
			poly[i] = t.apply(q)
		}
		fp.addPositive(poly)
		fp.fill(r.dst, hexColor(fill))
	}
	strokeClosed(&p, t, outline, width)
	p.fill(r.dst, stroke)
}

func endpoints(el Element, data map[string]any) (point, point) {
	x, y := f(el, "x"), f(el, "y")
	if x1, ok := data["x1"].(float64); ok {
		return point{x + x1, y + data["y1"].(float64)}, point{x + data["x2"].(float64), y + data["y2"].(float64)}
	}
	return point{x, y}, point{x + f(el, "w"), y + f(el, "h")}
}

// arrowHead is the two barb tips: length max(10, 3·width) at ±π/7.
func arrowHead(start, end point, width float64) (point, point) {
	angle := math.Atan2(end.y-start.y, end.x-start.x)
	l := math.Max(10, width*3)
	return point{end.x - l*math.Cos(angle-arrowHeadAngle), end.y - l*math.Sin(angle-arrowHeadAngle)},
		point{end.x - l*math.Cos(angle+arrowHeadAngle), end.y - l*math.Sin(angle+arrowHeadAngle)}
}

func (r *renderer) text(el Element, data map[string]any, t xform) {
	style := textStyle{font: r.fonts.regular, fallback: r.fonts.fallback, size: data["fontSize"].(float64)}
	if data["fontFamily"] == "mono" {
		style.font = r.fonts.mono
	}
	lineH := style.size * textLineHeight
	x, y := f(el, "x"), f(el, "y")
	base := style.baselineOffset(r.buf)
	c := hexColor(data["color"].(string))
	for i, line := range style.wrap(r.buf, data["text"].(string), f(el, "w")) {
		style.drawLine(r.dst, r.buf, t, line, x, y+float64(i)*lineH+lineH/2+base, c)
	}
}

func (r *renderer) image(el Element, data map[string]any, t xform) {
	dataURL := data["dataUrl"].(string)
	raw, err := base64.StdEncoding.DecodeString(dataURL[strings.Index(dataURL, ",")+1:])
	if err != nil {
		return
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return
	}
	nb := src.Bounds()
	nw, nh := float64(nb.Dx()), float64(nb.Dy())
	x, y, w, h := f(el, "x"), f(el, "y"), f(el, "w"), f(el, "h")
	// Fitted, aspect kept, centred (geometry.ts imageFit).
	s := math.Min(w/math.Max(nw, 1), h/math.Max(nh, 1))
	fx, fy := x+(w-nw*s)/2, y+(h-nh*s)/2
	// Source pixel (u, v) -> canvas (fx + u·s, fy + v·s) -> rotated -> output.
	o := t.apply(point{fx, fy})
	ux := t.apply(point{fx + s, fy})
	vy := t.apply(point{fx, fy + s})
	m := f64.Aff3{
		ux.x - o.x, vy.x - o.x, o.x - (ux.x-o.x)*float64(nb.Min.X) - (vy.x-o.x)*float64(nb.Min.Y),
		ux.y - o.y, vy.y - o.y, o.y - (ux.y-o.y)*float64(nb.Min.X) - (vy.y-o.y)*float64(nb.Min.Y),
	}
	xdraw.CatmullRom.Transform(r.dst, m, src, nb, xdraw.Over, nil)
}

// refCard is the card-coloured rounded box with the item's icon and title
// (at most two lines, ellipsised), vertically centred -- the title-only
// card. A status ("Deleted", "In trash") is folded into the title.
func (r *renderer) refCard(el Element, kind, id string, t xform) {
	x, y, w, h := f(el, "x"), f(el, "y"), f(el, "w"), f(el, "h")
	title, status := genericRefLabel(kind), ""
	if r.opt.RefText != nil {
		title, status = r.opt.RefText(kind, id)
	}
	if status != "" {
		title += " (" + status + ")"
	}
	pal := r.opt.Palette
	if pal.Card == nil {
		pal = LightPalette
	}

	var box path
	box.addPositive(roundedRect(t, x, y, w, h, cardRadius))
	box.fill(r.dst, pal.Card)
	var border path
	outline := roundedRectPoints(x+0.5, y+0.5, math.Max(w-1, 0), math.Max(h-1, 0), cardRadius)
	strokeClosed(&border, t, outline, 1)
	mr, mg, mb, _ := pal.Muted.RGBA()
	border.fill(r.dst, color.NRGBA{uint8(mr >> 8), uint8(mg >> 8), uint8(mb >> 8), 77})

	icon := noteIconPath
	if kind == "task" {
		icon = taskIconPath
	}
	var ip path
	addSVGPath(&ip, t, icon, x+cardPadding, y+h/2-cardIcon/2.0, cardIcon/24.0)
	ip.fill(r.dst, pal.Foreground)

	style := textStyle{font: r.fonts.medium, fallback: r.fonts.fallback, size: cardTitleSize}
	textLeft := x + cardPadding + cardIcon + cardGap
	textWidth := math.Max(w-(cardPadding+cardIcon+cardGap)-cardPadding, 1)
	lines := style.wrap(r.buf, title, textWidth)
	if len(lines) > 2 {
		lines = lines[:2]
		lines[1] = style.ellipsize(r.buf, lines[1]+" …", textWidth)
	}
	lineH := cardTitleSize * textLineHeight
	top := y + h/2 - float64(len(lines))*lineH/2
	base := style.baselineOffset(r.buf)
	for i, line := range lines {
		style.drawLine(r.dst, r.buf, t, line, textLeft, top+float64(i)*lineH+lineH/2+base, pal.Foreground)
	}
}

// ellipsize trims text to fit width, ending in "…".
func (s textStyle) ellipsize(buf *sfnt.Buffer, text string, width float64) string {
	text = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(text, "…")), " ")
	for s.width(buf, text+"…") > width && text != "" {
		_, size := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-size]
	}
	return strings.TrimRight(text, " ") + "…"
}

func genericRefLabel(kind string) string {
	if kind == "note" {
		return "Note"
	}
	return "Task"
}

func roundedRectPoints(x, y, w, h, rad float64) []point {
	rad = math.Min(rad, math.Min(w, h)/2)
	var pts []point
	corner := func(cx, cy, from float64) {
		for i := 0; i <= 6; i++ {
			a := from + float64(i)*math.Pi/2/6
			pts = append(pts, point{cx + rad*math.Cos(a), cy + rad*math.Sin(a)})
		}
	}
	corner(x+w-rad, y+rad, -math.Pi/2)
	corner(x+w-rad, y+h-rad, 0)
	corner(x+rad, y+h-rad, math.Pi/2)
	corner(x+rad, y+rad, math.Pi)
	return pts
}

func roundedRect(t xform, x, y, w, h, rad float64) []point {
	pts := roundedRectPoints(x, y, w, h, rad)
	for i, q := range pts {
		pts[i] = t.apply(q)
	}
	return pts
}

// contentBounds is the axis-aligned bounds of every element's rotated box.
func contentBounds(elements []Element) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, el := range elements {
		x, y, w, h := f(el, "x"), f(el, "y"), f(el, "w"), f(el, "h")
		cx, cy := x+w/2, y+h/2
		rad := f(el, "rotation") * math.Pi / 180
		c, s := math.Abs(math.Cos(rad)), math.Abs(math.Sin(rad))
		hw, hh := (w*c+h*s)/2, (w*s+h*c)/2
		minX, minY = math.Min(minX, cx-hw), math.Min(minY, cy-hh)
		maxX, maxY = math.Max(maxX, cx+hw), math.Max(maxY, cy+hh)
	}
	return
}

func f(el Element, key string) float64 {
	v, _ := el[key].(float64)
	return v
}

func hexColor(s string) color.Color {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return color.Black
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// ParseColor reads "#RRGGBB" for --background.
func ParseColor(s string) (color.Color, error) {
	if !colorPattern.MatchString(s) {
		return nil, fmt.Errorf("%q isn't a #RRGGBB colour", s)
	}
	return hexColor(s), nil
}
