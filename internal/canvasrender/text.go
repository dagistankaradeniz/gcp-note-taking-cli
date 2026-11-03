package canvasrender

import (
	_ "embed"
	"image"
	"image/color"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Fonts: Lexend (the app font, SIL OFL 1.1 -- see fonts/LEXEND_OFL.txt)
// for sans text and ref cards, Go Mono for monospace. The CLI has no
// serif face, so serif text falls back to Lexend. Characters Lexend lacks
// fall back to Go Regular (wider Latin/Greek/Cyrillic coverage); scripts
// neither covers (e.g. CJK, Arabic) can't be drawn and show as boxes.

//go:embed fonts/Lexend-Regular.ttf
var lexendRegular []byte

//go:embed fonts/Lexend-Medium.ttf
var lexendMedium []byte

type fontSet struct {
	regular, medium, mono, fallback *sfnt.Font
}

var (
	fontsOnce sync.Once
	fonts     fontSet
	fontsErr  error
)

func loadFonts() (fontSet, error) {
	fontsOnce.Do(func() {
		parse := func(b []byte) *sfnt.Font {
			if fontsErr != nil {
				return nil
			}
			f, err := sfnt.Parse(b)
			if err != nil {
				fontsErr = err
			}
			return f
		}
		fonts = fontSet{
			regular:  parse(lexendRegular),
			medium:   parse(lexendMedium),
			mono:     parse(gomono.TTF),
			fallback: parse(goregular.TTF),
		}
	})
	return fonts, fontsErr
}

// textStyle is a face at a size in canvas units.
type textStyle struct {
	font     *sfnt.Font
	fallback *sfnt.Font
	size     float64
}

func (s textStyle) ppem() fixed.Int26_6 { return fixed.Int26_6(s.size * 64) }

// glyph resolves r to a font and glyph index, falling back when the main
// font has no glyph for it.
func (s textStyle) glyph(buf *sfnt.Buffer, r rune) (*sfnt.Font, sfnt.GlyphIndex) {
	if idx, err := s.font.GlyphIndex(buf, r); err == nil && idx != 0 {
		return s.font, idx
	}
	if s.fallback != nil {
		if idx, err := s.fallback.GlyphIndex(buf, r); err == nil && idx != 0 {
			return s.fallback, idx
		}
	}
	idx, _ := s.font.GlyphIndex(buf, r)
	return s.font, idx
}

func (s textStyle) advance(buf *sfnt.Buffer, r rune) float64 {
	f, idx := s.glyph(buf, r)
	adv, err := f.GlyphAdvance(buf, idx, s.ppem(), font.HintingNone)
	if err != nil {
		return 0
	}
	return float64(adv) / 64
}

func (s textStyle) width(buf *sfnt.Buffer, text string) float64 {
	w := 0.0
	for _, r := range text {
		w += s.advance(buf, r)
	}
	return w
}

// baselineOffset is where the alphabetic baseline sits below the middle
// of a line -- the canvas "middle" text baseline Konva draws with.
func (s textStyle) baselineOffset(buf *sfnt.Buffer) float64 {
	m, err := s.font.Metrics(buf, s.ppem(), font.HintingNone)
	if err != nil {
		return s.size * 0.35
	}
	ascent, descent := float64(m.Ascent)/64, float64(m.Descent)/64
	if ascent+descent <= 0 {
		return s.size * 0.35
	}
	return s.size * (ascent/(ascent+descent) - 0.5)
}

// wrap breaks text into lines no wider than width, at spaces where it can
// and inside a word when the word alone is too wide (Konva's wrap="word").
func (s textStyle) wrap(buf *sfnt.Buffer, text string, width float64) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Split(para, " ")
		line := ""
		for i, w := range words {
			candidate := w
			if i > 0 {
				candidate = line + " " + w
			}
			if i == 0 || s.width(buf, candidate) <= width {
				line = candidate
			} else {
				lines = append(lines, line)
				line = w
			}
			for s.width(buf, line) > width && utf8.RuneCountInString(line) > 1 {
				head, rest := s.splitToWidth(buf, line, width)
				lines = append(lines, head)
				line = rest
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// splitToWidth cuts the longest prefix (at least one character) that fits.
func (s textStyle) splitToWidth(buf *sfnt.Buffer, text string, width float64) (string, string) {
	w := 0.0
	for i, r := range text {
		w += s.advance(buf, r)
		if w > width && i > 0 {
			return text[:i], text[i:]
		}
	}
	return text, ""
}

// drawLine draws one line of text with its baseline at (x, y), canvas
// units, through t.
func (s textStyle) drawLine(dst *image.RGBA, buf *sfnt.Buffer, t xform, text string, x, y float64, c color.Color) {
	var p path
	pen := x
	for _, r := range text {
		f, idx := s.glyph(buf, r)
		segs, err := f.LoadGlyph(buf, idx, s.ppem(), nil)
		if err == nil {
			addGlyph(&p, t, segs, pen, y)
		}
		adv, err := f.GlyphAdvance(buf, idx, s.ppem(), font.HintingNone)
		if err == nil {
			pen += float64(adv) / 64
		}
	}
	p.fill(dst, c)
}

// addGlyph flattens a glyph outline (y down, 26.6 canvas units relative
// to the pen) into polygons.
func addGlyph(p *path, t xform, segs sfnt.Segments, ox, oy float64) {
	at := func(q fixed.Point26_6) point {
		return t.apply(point{ox + float64(q.X)/64, oy + float64(q.Y)/64})
	}
	var cur []point
	var last point
	flush := func() {
		p.add(cur)
		cur = nil
	}
	for _, seg := range segs {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			flush()
			last = at(seg.Args[0])
			cur = []point{last}
		case sfnt.SegmentOpLineTo:
			last = at(seg.Args[0])
			cur = append(cur, last)
		case sfnt.SegmentOpQuadTo:
			c1, end := at(seg.Args[0]), at(seg.Args[1])
			for i := 1; i <= 8; i++ {
				u := float64(i) / 8
				cur = append(cur, quad(last, c1, end, u))
			}
			last = end
		case sfnt.SegmentOpCubeTo:
			c1, c2, end := at(seg.Args[0]), at(seg.Args[1]), at(seg.Args[2])
			for i := 1; i <= 10; i++ {
				u := float64(i) / 10
				cur = append(cur, cube(last, c1, c2, end, u))
			}
			last = end
		}
	}
	flush()
}

func quad(a, b, c point, u float64) point {
	v := 1 - u
	return point{v*v*a.x + 2*v*u*b.x + u*u*c.x, v*v*a.y + 2*v*u*b.y + u*u*c.y}
}

func cube(a, b, c, d point, u float64) point {
	v := 1 - u
	return point{
		v*v*v*a.x + 3*v*v*u*b.x + 3*v*u*u*c.x + u*u*u*d.x,
		v*v*v*a.y + 3*v*v*u*b.y + 3*v*u*u*c.y + u*u*u*d.y,
	}
}
