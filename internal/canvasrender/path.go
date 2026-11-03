package canvasrender

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

type point struct{ x, y float64 }

// xform maps canvas units to output pixels: rotate about the element's
// centre (its rotation), then shift by the export origin and scale.
type xform struct {
	cx, cy, cos, sin float64
	ox, oy, scale    float64
}

func (t xform) apply(p point) point {
	dx, dy := p.x-t.cx, p.y-t.cy
	rx := t.cx + dx*t.cos - dy*t.sin
	ry := t.cy + dx*t.sin + dy*t.cos
	return point{(rx - t.ox) * t.scale, (ry - t.oy) * t.scale}
}

// path collects closed sub-paths in output pixels. Strokes are built as
// unions of positively-wound polygons, so overlaps (round joins over
// segments) never cancel; glyphs and icons keep their own winding, which
// is what cuts their holes.
type path struct {
	subs [][]point
}

func (p *path) add(poly []point) {
	if len(poly) >= 3 {
		p.subs = append(p.subs, poly)
	}
}

// addPositive adds poly wound so its signed area is positive.
func (p *path) addPositive(poly []point) {
	if signedArea(poly) < 0 {
		for i, j := 0, len(poly)-1; i < j; i, j = i+1, j-1 {
			poly[i], poly[j] = poly[j], poly[i]
		}
	}
	p.add(poly)
}

func signedArea(poly []point) float64 {
	a := 0.0
	for i := range poly {
		j := (i + 1) % len(poly)
		a += poly[i].x*poly[j].y - poly[j].x*poly[i].y
	}
	return a / 2
}

// fill rasterises the path into dst with c, only over the path's own
// pixel bounds (a full-canvas rasteriser per element would be far too slow
// on big exports).
func (p *path) fill(dst *image.RGBA, c color.Color) {
	if len(p.subs) == 0 {
		return
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, s := range p.subs {
		for _, q := range s {
			minX, minY = math.Min(minX, q.x), math.Min(minY, q.y)
			maxX, maxY = math.Max(maxX, q.x), math.Max(maxY, q.y)
		}
	}
	b := dst.Bounds()
	r := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).Intersect(b)
	if r.Empty() {
		return
	}
	z := vector.NewRasterizer(r.Dx(), r.Dy())
	z.DrawOp = draw.Over
	ox, oy := float32(r.Min.X), float32(r.Min.Y)
	for _, s := range p.subs {
		z.MoveTo(float32(s[0].x)-ox, float32(s[0].y)-oy)
		for _, q := range s[1:] {
			z.LineTo(float32(q.x)-ox, float32(q.y)-oy)
		}
		z.ClosePath()
	}
	z.Draw(dst, r, image.NewUniform(c), image.Point{})
}

// circlePoly is a circle of radius r (canvas units) around c, transformed.
func circlePoly(t xform, c point, r float64) []point {
	n := segmentsFor(r * t.scale)
	poly := make([]point, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		poly[i] = t.apply(point{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
	return poly
}

// ellipsePoints is the outline of an axis-aligned ellipse (canvas units).
func ellipsePoints(cx, cy, rx, ry, scale float64) []point {
	n := segmentsFor(math.Max(rx, ry) * scale)
	pts := make([]point, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = point{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return pts
}

// segmentsFor picks a polygon resolution for a radius in pixels.
func segmentsFor(radiusPx float64) int {
	n := int(radiusPx * 1.5)
	return max(16, min(n, 256))
}

// strokeSegment adds a round-capped stroke from a to b (canvas units).
func strokeSegment(p *path, t xform, a, b point, width float64) {
	r := width / 2
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l > 0 {
		nx, ny := -dy/l*r, dx/l*r
		p.addPositive([]point{
			t.apply(point{a.x + nx, a.y + ny}),
			t.apply(point{b.x + nx, b.y + ny}),
			t.apply(point{b.x - nx, b.y - ny}),
			t.apply(point{a.x - nx, a.y - ny}),
		})
	}
	p.addPositive(circlePoly(t, a, r))
	p.addPositive(circlePoly(t, b, r))
}

// strokeClosed strokes a closed polyline with round joins.
func strokeClosed(p *path, t xform, pts []point, width float64) {
	for i := range pts {
		strokeSegment(p, t, pts[i], pts[(i+1)%len(pts)], width)
	}
}
