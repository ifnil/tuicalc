package tui

import (
	"math"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"

	"tcalc/expr"
)

// viewport describes which part of the plane is visible. Scales are in
// units per braille dot; dots are roughly square on most terminal fonts,
// so sx == sy gives an undistorted graph.
type viewport struct {
	cx, cy float64 // center of the view
	sx, sy float64 // units per dot horizontally and vertically
}

const (
	minScale  = 1e-9
	maxScale  = 1e9
	maxCenter = 1e12
)

func (v *viewport) clamp() {
	v.sx = math.Min(math.Max(v.sx, minScale), maxScale)
	v.sy = math.Min(math.Max(v.sy, minScale), maxScale)
	v.cx = math.Min(math.Max(v.cx, -maxCenter), maxCenter)
	v.cy = math.Min(math.Max(v.cy, -maxCenter), maxCenter)
}

// zoomAt scales the view by factor while keeping (x, y) fixed on screen.
func (v *viewport) zoomAt(x, y, fx, fy float64) {
	v.sx *= fx
	v.sy *= fy
	v.cx = x + (v.cx-x)*fx
	v.cy = y + (v.cy-y)*fy
	v.clamp()
}

// frame is a linechart sized for the plot area, with tick labels placed at
// round numbers, plus the mapping between plane coordinates and braille
// dots.
//
// The linechart draws the left and bottom axes and their labels. The graph
// area lies to the right of / above them and is W×H cells, i.e. 2W×4H
// braille dots. Dot columns grow rightwards and dot rows grow downwards.
type frame struct {
	vp     viewport
	ox, oy int // canvas position of the axes corner
	W, H   int // plot area in cells

	xStep, yStep float64
	xTicks       []int // plot-area cell columns holding an x tick
	yTicks       []int // plot-area cell rows holding a y tick
	xLabels      map[int]string
	yLabels      map[int]string

	chart linechart.Model
}

func newFrame(w, h int, vp viewport, th *theme) *frame {
	f := &frame{vp: vp, H: h - 2, xLabels: map[int]string{}, yLabels: map[int]string{}}
	f.layoutYTicks()
	f.chart = linechart.New(w, h, 0, 1, 0, 1,
		linechart.WithXYSteps(1, 1),
		linechart.WithStyles(th.axis, th.label, lipgloss.NewStyle()),
		linechart.WithXLabelFormatter(func(i int, _ float64) string { return f.xLabels[i] }),
		linechart.WithYLabelFormatter(func(i int, _ float64) string { return f.yLabels[i] }),
	)
	o := f.chart.Origin()
	f.ox, f.oy = o.X, o.Y
	f.W = f.chart.GraphWidth()
	f.layoutXTicks()
	return f
}

func (f *frame) dotsW() int { return 2 * f.W }
func (f *frame) dotsH() int { return 4 * f.H }

func (f *frame) xToDot(x float64) float64 {
	return (x-f.vp.cx)/f.vp.sx + float64(f.dotsW()-1)/2
}

func (f *frame) dotToX(d float64) float64 {
	return f.vp.cx + (d-float64(f.dotsW()-1)/2)*f.vp.sx
}

func (f *frame) yToDot(y float64) float64 {
	return float64(f.dotsH()-1)/2 - (y-f.vp.cy)/f.vp.sy
}

func (f *frame) dotToY(r float64) float64 {
	return f.vp.cy + (float64(f.dotsH()-1)/2-r)*f.vp.sy
}

// xRange and yRange return the visible extent of the plane.
func (f *frame) xRange() (float64, float64) {
	return f.dotToX(-0.5), f.dotToX(float64(f.dotsW()) - 0.5)
}

func (f *frame) yRange() (float64, float64) {
	return f.dotToY(float64(f.dotsH()) - 0.5), f.dotToY(-0.5)
}

// col returns the plot-area cell column that contains x.
func (f *frame) col(x float64) (int, bool) {
	d := math.Round(f.xToDot(x))
	if d < 0 || d >= float64(f.dotsW()) {
		return 0, false
	}
	return int(d) / 2, true
}

// row returns the plot-area cell row that contains y.
func (f *frame) row(y float64) (int, bool) {
	r := math.Round(f.yToDot(y))
	if r < 0 || r >= float64(f.dotsH()) {
		return 0, false
	}
	return int(r) / 4, true
}

// cell returns the plot-area cell that contains (x, y).
func (f *frame) cell(x, y float64) (j, k int, ok bool) {
	j, okx := f.col(x)
	k, oky := f.row(y)
	return j, k, okx && oky
}

// canvasCell converts a canvas position to a plot-area cell.
func (f *frame) canvasCell(cx, cy int) (j, k int, ok bool) {
	j, k = cx-f.ox-1, cy
	return j, k, j >= 0 && j < f.W && k >= 0 && k < f.H
}

// cellCenter returns the plane coordinates at the middle of a plot cell.
func (f *frame) cellCenter(j, k int) (float64, float64) {
	return f.dotToX(float64(2*j) + 0.5), f.dotToY(float64(4*k) + 1.5)
}

func (f *frame) layoutYTicks() {
	if f.H <= 0 {
		return
	}
	f.yStep = expr.NiceStep(4 * f.vp.sy * 3) // at least three rows apart
	lo, hi := f.yRange()
	for n := math.Ceil(lo / f.yStep); n <= math.Floor(hi/f.yStep); n++ {
		t := n * f.yStep
		k, ok := f.row(t)
		if !ok {
			continue
		}
		f.yTicks = append(f.yTicks, k)
		// The linechart labels rows by their distance above the x axis.
		f.yLabels[f.H-k] = expr.FormatTick(t, f.yStep)
	}
}

func (f *frame) layoutXTicks() {
	if f.W <= 0 {
		return
	}
	perCell := 2 * f.vp.sx
	lo, hi := f.xRange()
	step := expr.NiceStep(perCell * 6)
	for range 8 {
		width := max(len(expr.FormatTick(math.Floor(lo/step)*step, step)),
			len(expr.FormatTick(math.Ceil(hi/step)*step, step)))
		if step/perCell >= float64(width+2) {
			break
		}
		step = expr.NiceStep(step * 1.01)
	}
	f.xStep = step
	for n := math.Ceil(lo / step); n <= math.Floor(hi/step); n++ {
		t := n * step
		j, ok := f.col(t)
		if !ok {
			continue
		}
		f.xTicks = append(f.xTicks, j)
		// Labels are drawn left-anchored from a column offset relative to
		// the y axis; center them on the tick.
		label := expr.FormatTick(t, step)
		f.xLabels[max(0, j+1-len(label)/2)] = label
	}
}

type curve struct {
	fn    func(float64) float64
	vline bool
	x     float64
	style lipgloss.Style
}

type marker struct {
	x, y  float64
	style lipgloss.Style
}

func (f *frame) render(th *theme, curves []curve, showGrid bool, mk *marker) string {
	f.chart.DrawXYAxisAndLabel()
	f.drawGuides(th, showGrid)
	origin := canvas.Point{X: f.ox + 1, Y: 0}
	for _, c := range curves {
		g := graph.NewBrailleGrid(f.W, f.H, 0, 1, 0, 1)
		if c.vline {
			f.plotVLine(g, c.x)
		} else {
			p := plotter{f: f, fn: c.fn, grid: g, budget: 60 * f.dotsW(),
				dw: float64(f.dotsW()), dh: float64(f.dotsH())}
			p.run()
		}
		graph.DrawBraillePatterns(&f.chart.Canvas, origin, g.BraillePatterns(), c.style)
	}
	if mk != nil {
		if j, k, ok := f.cell(mk.x, mk.y); ok {
			f.chart.Canvas.SetCell(canvas.Point{X: f.ox + 1 + j, Y: k},
				canvas.NewCellWithStyle('●', mk.style.Bold(true)))
		}
	}
	return f.chart.View()
}

// drawGuides draws the x = 0 and y = 0 axes with tick marks and, if
// enabled, a dot grid at tick intersections. Curves are drawn on top.
func (f *frame) drawGuides(th *theme, showGrid bool) {
	cv := &f.chart.Canvas
	set := func(j, k int, r rune, s lipgloss.Style) {
		cv.SetCell(canvas.Point{X: f.ox + 1 + j, Y: k}, canvas.NewCellWithStyle(r, s))
	}
	if showGrid {
		for _, j := range f.xTicks {
			for _, k := range f.yTicks {
				set(j, k, '·', th.grid)
			}
		}
	}
	j0, xAxis := f.col(0)
	k0, yAxis := f.row(0)
	if yAxis {
		for j := range f.W {
			set(j, k0, '─', th.zeroAxis)
		}
		for _, j := range f.xTicks {
			set(j, k0, '┼', th.zeroAxis)
		}
	}
	if xAxis {
		for k := range f.H {
			set(j0, k, '│', th.zeroAxis)
		}
		for _, k := range f.yTicks {
			set(j0, k, '┼', th.zeroAxis)
		}
	}
	if xAxis && yAxis {
		set(j0, k0, '┼', th.zeroAxis)
	}
}

func (f *frame) plotVLine(g *graph.BrailleGrid, x float64) {
	d := math.Round(f.xToDot(x))
	if d < 0 || d >= float64(f.dotsW()) {
		return
	}
	for r := range f.dotsH() {
		g.Set(canvas.Point{X: int(d), Y: r})
	}
}

// plotter draws y = fn(x) into a braille grid. It samples once per dot
// column and adaptively bisects steep segments, which keeps near-vertical
// parts connected while leaving gaps at asymptotes and jumps.
type plotter struct {
	f      *frame
	fn     func(float64) float64
	grid   *graph.BrailleGrid
	budget int
	dw, dh float64
}

const maxDepth = 10

// eval returns the dot row of the curve at dot column d, or NaN.
func (p *plotter) eval(d float64) float64 {
	p.budget--
	y := p.fn(p.f.dotToX(d))
	if math.IsNaN(y) || math.IsInf(y, 0) {
		return math.NaN()
	}
	return p.f.yToDot(y)
}

func (p *plotter) run() {
	prevD := -1.0
	prevR := p.eval(prevD)
	for i := 0; i <= int(p.dw); i++ {
		d := float64(i)
		r := p.eval(d)
		p.segment(prevD, prevR, d, r, 0)
		prevD, prevR = d, r
	}
}

func (p *plotter) segment(d0, r0, d1, r1 float64, depth int) {
	ok0, ok1 := !math.IsNaN(r0), !math.IsNaN(r1)
	switch {
	case !ok0 && !ok1:
		return
	case ok0 && ok1:
		if (r0 < -1 && r1 < -1) || (r0 > p.dh && r1 > p.dh) {
			return // entirely above or below the view
		}
		jump := math.Abs(r1 - r0)
		if jump <= 1 {
			p.line(d0, r0, d1, r1)
			return
		}
		if depth >= maxDepth || p.budget <= 0 {
			// A continuous curve this steep over 1/1024 of a dot is
			// vanishingly rare, so treat larger jumps as discontinuities.
			if jump <= 2 || p.budget <= 0 {
				p.line(d0, r0, d1, r1)
			}
			return
		}
	default:
		// One end is undefined: home in on the edge of the domain.
		if depth >= maxDepth || p.budget <= 0 {
			if ok0 {
				p.dot(d0, r0)
			} else {
				p.dot(d1, r1)
			}
			return
		}
	}
	dm := (d0 + d1) / 2
	rm := p.eval(dm)
	if ok0 && ok1 && !math.IsNaN(rm) && math.Abs(r1-r0) < p.dh && math.Abs(rm-(r0+r1)/2) <= 0.5 {
		p.line(d0, r0, d1, r1) // straight enough
		return
	}
	p.segment(d0, r0, dm, rm, depth+1)
	p.segment(dm, rm, d1, r1, depth+1)
}

func (p *plotter) dot(d, r float64) {
	d, r = math.Round(d), math.Round(r)
	if d >= 0 && r >= 0 && d < p.dw && r < p.dh {
		p.grid.Set(canvas.Point{X: int(d), Y: int(r)})
	}
}

func (p *plotter) line(d0, r0, d1, r1 float64) {
	d0, r0, d1, r1, ok := clipLine(d0, r0, d1, r1, -0.49, -0.49, p.dw-0.51, p.dh-0.51)
	if !ok {
		return
	}
	a := canvas.Point{X: int(math.Round(d0)), Y: int(math.Round(r0))}
	b := canvas.Point{X: int(math.Round(d1)), Y: int(math.Round(r1))}
	p.grid.Set(a)
	p.grid.Set(b)
	for _, pt := range graph.GetLinePoints(a, b) {
		p.grid.Set(pt)
	}
}

// clipLine clips a segment to a rectangle (Liang–Barsky).
func clipLine(x0, y0, x1, y1, xmin, ymin, xmax, ymax float64) (float64, float64, float64, float64, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := x1-x0, y1-y0
	edges := [4][2]float64{{-dx, x0 - xmin}, {dx, xmax - x0}, {-dy, y0 - ymin}, {dy, ymax - y0}}
	for _, e := range edges {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return 0, 0, 0, 0, false
			}
			t0 = math.Max(t0, t)
		} else {
			if t < t0 {
				return 0, 0, 0, 0, false
			}
			t1 = math.Min(t1, t)
		}
	}
	return x0 + t0*dx, y0 + t0*dy, x0 + t1*dx, y0 + t1*dy, true
}
