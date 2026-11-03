package canvasrender

import (
	"strconv"
	"unicode"
)

// addSVGPath flattens the small SVG path subset the ref-card icons use
// (M L H V C Z, absolute and relative, implicit repeats) into p, placing
// the 24-unit icon at (x, y) scaled by s, then through t.
func addSVGPath(p *path, t xform, d string, x, y, s float64) {
	nums := func(i int) ([]float64, int) {
		var out []float64
		for i < len(d) {
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			if i >= len(d) || unicode.IsLetter(rune(d[i])) {
				break
			}
			j, dot := i, false
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			for j < len(d) && (d[j] >= '0' && d[j] <= '9' || d[j] == '.' && !dot) {
				if d[j] == '.' {
					dot = true
				}
				j++
			}
			v, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil || j == i {
				break
			}
			out = append(out, v)
			i = j
		}
		return out, i
	}
	at := func(q point) point { return t.apply(point{x + q.x*s, y + q.y*s}) }

	var cur []point
	var pos, start point
	flush := func() {
		p.add(cur)
		cur = nil
	}
	lineTo := func(q point) {
		pos = q
		cur = append(cur, at(q))
	}
	for i := 0; i < len(d); {
		cmd := d[i]
		if !unicode.IsLetter(rune(cmd)) {
			i++
			continue
		}
		args, next := nums(i + 1)
		i = next
		rel := cmd >= 'a'
		off := func() point {
			if rel {
				return pos
			}
			return point{}
		}
		switch cmd | 0x20 {
		case 'm':
			for k := 0; k+1 < len(args); k += 2 {
				o := off()
				q := point{o.x + args[k], o.y + args[k+1]}
				if k == 0 {
					flush()
					start = q
					pos = q
					cur = []point{at(q)}
				} else {
					lineTo(q)
				}
			}
		case 'l':
			for k := 0; k+1 < len(args); k += 2 {
				o := off()
				lineTo(point{o.x + args[k], o.y + args[k+1]})
			}
		case 'h':
			for _, v := range args {
				if rel {
					v += pos.x
				}
				lineTo(point{v, pos.y})
			}
		case 'v':
			for _, v := range args {
				if rel {
					v += pos.y
				}
				lineTo(point{pos.x, v})
			}
		case 'c':
			for k := 0; k+5 < len(args); k += 6 {
				o := off()
				a := pos
				c1 := point{o.x + args[k], o.y + args[k+1]}
				c2 := point{o.x + args[k+2], o.y + args[k+3]}
				end := point{o.x + args[k+4], o.y + args[k+5]}
				for n := 1; n <= 8; n++ {
					cur = append(cur, at(cube(a, c1, c2, end, float64(n)/8)))
				}
				pos = end
			}
		case 'z':
			flush()
			pos = start
		}
	}
	flush()
}
