package query

import (
	"fmt"
	"math"
	"sort"

	"glucolens/internal/dataset"
)

// CorrColumn identifies a column in the correlation matrix.
type CorrColumn struct {
	Name    string       `json:"name"`
	Label   string       `json:"label"`
	Short   string       `json:"short"`
	Section string       `json:"section"`
	Kind    dataset.Kind `json:"kind"`
}

// CorrPair is one off-diagonal entry, used to list the strongest links.
type CorrPair struct {
	A string  `json:"a"`
	B string  `json:"b"`
	R float64 `json:"r"`
}

// CorrelationResponse is the Pearson correlation matrix of numeric columns.
type CorrelationResponse struct {
	Columns []CorrColumn `json:"columns"`
	Matrix  [][]float64  `json:"matrix"`
	N       int          `json:"n"`
	Top     []CorrPair   `json:"top"`
}

// Correlations returns the correlation matrix over the whole dataset. It is
// computed once and cached.
func (e *Engine) Correlations() *CorrelationResponse {
	e.corrOnce.Do(func() { e.corr = e.computeCorrelations() })
	return e.corr
}

func (e *Engine) computeCorrelations() *CorrelationResponse {
	var cols []*dataset.Column
	for _, c := range e.ds.Columns {
		if c.IsNumeric() {
			cols = append(cols, c)
		}
	}
	resp := &CorrelationResponse{N: e.ds.Rows}
	for _, c := range cols {
		resp.Columns = append(resp.Columns, CorrColumn{
			Name: c.Name, Label: c.Label, Short: c.Short, Section: c.Section, Kind: c.Kind,
		})
	}
	n := len(cols)
	resp.Matrix = make([][]float64, n)
	for i := range resp.Matrix {
		resp.Matrix[i] = make([]float64, n)
		resp.Matrix[i][i] = 1
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			r := round(pearson(cols[i], cols[j], e.allRows), 4)
			resp.Matrix[i][j], resp.Matrix[j][i] = r, r
			resp.Top = append(resp.Top, CorrPair{A: cols[i].Name, B: cols[j].Name, R: r})
		}
	}
	sort.Slice(resp.Top, func(a, b int) bool { return math.Abs(resp.Top[a].R) > math.Abs(resp.Top[b].R) })
	if len(resp.Top) > 12 {
		resp.Top = resp.Top[:12]
	}
	return resp
}

// pearson computes the correlation over rows where both values exist.
func pearson(a, b *dataset.Column, rows []int32) float64 {
	var n, sx, sy, sxx, syy, sxy float64
	for _, r := range rows {
		x, y := a.Num(int(r)), b.Num(int(r))
		if math.IsNaN(x) || math.IsNaN(y) {
			continue
		}
		n++
		sx += x
		sy += y
		sxx += x * x
		syy += y * y
		sxy += x * y
	}
	if n < 3 {
		return 0
	}
	cov := sxy - sx*sy/n
	vx := sxx - sx*sx/n
	vy := syy - sy*sy/n
	if vx <= 0 || vy <= 0 {
		return 0
	}
	return finite(cov / math.Sqrt(vx*vy))
}

// RelationshipRequest asks how one numeric column varies with another.
type RelationshipRequest struct {
	X       string   `json:"x"`
	Y       string   `json:"y"`
	Filters []Filter `json:"filters,omitempty"`
	XBins   int      `json:"xBins,omitempty"`
	YBins   int      `json:"yBins,omitempty"`
}

// AxisInfo describes one axis of a relationship chart.
type AxisInfo struct {
	Column   string       `json:"column"`
	Label    string       `json:"label"`
	Unit     string       `json:"unit,omitempty"`
	Kind     dataset.Kind `json:"kind"`
	Decimals int          `json:"decimals"`
}

// Cell is a non-empty cell of the 2-D density grid.
type Cell struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Count int `json:"count"`
}

// TrendPoint summarises Y within one X bin.
type TrendPoint struct {
	Bin    int     `json:"bin"`
	X      float64 `json:"x"`
	N      int     `json:"n"`
	Mean   float64 `json:"mean"`
	P25    float64 `json:"p25"`
	Median float64 `json:"median"`
	P75    float64 `json:"p75"`
}

// RelationshipResponse holds a density grid, a binned trend and a fit.
type RelationshipResponse struct {
	X         AxisInfo     `json:"x"`
	Y         AxisInfo     `json:"y"`
	Cohort    string       `json:"cohort"`
	N         int          `json:"n"`
	XBins     []Bin        `json:"xBins"`
	YBins     []Bin        `json:"yBins"`
	Cells     []Cell       `json:"cells"`
	MaxCount  int          `json:"maxCount"`
	Trend     []TrendPoint `json:"trend"`
	Pearson   float64      `json:"pearson"`
	Slope     float64      `json:"slope"`
	Intercept float64      `json:"intercept"`
	Summary   string       `json:"summary"`
}

// Relationship bins two numeric columns into a density grid and computes
// how Y changes across X.
func (e *Engine) Relationship(req RelationshipRequest) (*RelationshipResponse, error) {
	xc, err := e.column(req.X, "x column")
	if err != nil {
		return nil, err
	}
	yc, err := e.column(req.Y, "y column")
	if err != nil {
		return nil, err
	}
	if !xc.IsNumeric() || !yc.IsNumeric() {
		return nil, invalidf("relationships need two numeric columns; use compare for categories")
	}
	if xc.Name == yc.Name {
		return nil, invalidf("choose two different columns")
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}

	xb, yb := req.XBins, req.YBins
	if xb <= 0 {
		xb = 30
	}
	if yb <= 0 {
		yb = 24
	}
	xBins, xIndex := axisBins(xc, min(max(xb, 4), 60))
	yBins, yIndex := axisBins(yc, min(max(yb, 4), 60))

	grid := make([][]int, len(xBins))
	for i := range grid {
		grid[i] = make([]int, len(yBins))
	}
	perX := make([][]float64, len(xBins))
	var n, sx, sy, sxx, sxy, syy float64
	for _, r := range rows {
		i := int(r)
		x, y := xc.Num(i), yc.Num(i)
		if math.IsNaN(x) || math.IsNaN(y) {
			continue
		}
		xi, yi := xIndex(x), yIndex(y)
		grid[xi][yi]++
		perX[xi] = append(perX[xi], y)
		n++
		sx += x
		sy += y
		sxx += x * x
		syy += y * y
		sxy += x * y
	}

	resp := &RelationshipResponse{
		X:      AxisInfo{Column: xc.Name, Label: xc.Label, Unit: xc.Unit, Kind: xc.Kind, Decimals: xc.Decimals},
		Y:      AxisInfo{Column: yc.Name, Label: yc.Label, Unit: yc.Unit, Kind: yc.Kind, Decimals: yc.Decimals},
		Cohort: e.DescribeFilters(req.Filters),
		N:      int(n),
		XBins:  xBins,
		YBins:  yBins,
	}
	for xi, col := range grid {
		for yi, cnt := range col {
			if cnt > 0 {
				resp.Cells = append(resp.Cells, Cell{X: xi, Y: yi, Count: cnt})
				resp.MaxCount = max(resp.MaxCount, cnt)
			}
		}
	}
	for xi, ys := range perX {
		if len(ys) == 0 {
			continue
		}
		s := summarize(ys)
		resp.Trend = append(resp.Trend, TrendPoint{
			Bin: xi, X: (xBins[xi].Start + xBins[xi].End) / 2, N: s.N,
			Mean: s.Mean, P25: s.P25, Median: s.Median, P75: s.P75,
		})
	}
	if n >= 3 {
		vx := sxx - sx*sx/n
		vy := syy - sy*sy/n
		cov := sxy - sx*sy/n
		if vx > 0 && vy > 0 {
			resp.Pearson = round(cov/math.Sqrt(vx*vy), 4)
			resp.Slope = finite(cov / vx)
			resp.Intercept = finite(sy/n - resp.Slope*sx/n)
		}
	}
	resp.Summary = relationshipSummary(resp)
	return resp, nil
}

// axisBins returns histogram bins for a continuous column, or one bin per
// level for yes/no and discrete columns.
func axisBins(c *dataset.Column, target int) ([]Bin, func(float64) int) {
	if c.Kind != dataset.KindContinuous {
		bins := make([]Bin, len(c.Levels))
		for i := range c.Levels {
			v := c.LevelValue(i)
			bins[i] = Bin{Start: v - 0.5, End: v + 0.5}
		}
		return bins, func(v float64) int {
			for i := range bins {
				if v >= bins[i].Start && v < bins[i].End {
					return i
				}
			}
			if v < bins[0].Start {
				return 0
			}
			return len(bins) - 1
		}
	}
	return niceBins(c.Stats.Min, c.Stats.Max, target, c.Decimals)
}

func relationshipSummary(r *RelationshipResponse) string {
	abs := math.Abs(r.Pearson)
	var strength string
	switch {
	case abs < 0.1:
		return fmt.Sprintf("Little to no linear relationship between %s and %s (r = %.2f).", r.X.Label, lowerFirst(r.Y.Label), r.Pearson)
	case abs < 0.3:
		strength = "weak"
	case abs < 0.5:
		strength = "moderate"
	default:
		strength = "strong"
	}
	dir := "rises"
	if r.Pearson < 0 {
		dir = "falls"
	}
	return fmt.Sprintf("A %s relationship: %s tends to %s as %s increases (r = %.2f).",
		strength, r.Y.Label, map[string]string{"rises": "rise", "falls": "fall"}[dir], lowerFirst(r.X.Label), r.Pearson)
}
