package query

import (
	"math"

	"glucolens/internal/dataset"
)

// DistributionRequest asks how a column's values are spread.
type DistributionRequest struct {
	Column  string   `json:"column"`
	Filters []Filter `json:"filters,omitempty"`
	SplitBy string   `json:"splitBy,omitempty"`
	Bins    int      `json:"bins,omitempty"`
}

// Bin is a histogram bucket [Start, End).
type Bin struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// DistGroup holds counts for everyone or for one split group.
type DistGroup struct {
	Key     string    `json:"key"`
	Label   string    `json:"label"`
	N       int       `json:"n"`
	Counts  []int     `json:"counts"`
	Shares  []float64 `json:"shares"`
	Summary *Summary  `json:"summary,omitempty"`
	LowN    bool      `json:"lowN,omitempty"`
}

// BandShare is the share of people within one clinical band.
type BandShare struct {
	Label string   `json:"label"`
	Tone  string   `json:"tone"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	Count int      `json:"count"`
	Share float64  `json:"share"`
}

// DistributionResponse is a histogram (numeric) or level counts.
type DistributionResponse struct {
	Column       string       `json:"column"`
	Label        string       `json:"label"`
	Kind         dataset.Kind `json:"kind"`
	Mode         string       `json:"mode"` // histogram | levels
	Bins         []Bin        `json:"bins,omitempty"`
	Levels       []string     `json:"levels,omitempty"`
	Cohort       string       `json:"cohort"`
	N            int          `json:"n"`
	Overall      DistGroup    `json:"overall"`
	SplitBy      string       `json:"splitBy,omitempty"`
	SplitByLabel string       `json:"splitByLabel,omitempty"`
	Groups       []DistGroup  `json:"groups,omitempty"`
	Bands        []BandShare  `json:"bands,omitempty"`
}

// Distribution counts a column's values, optionally split by groups.
func (e *Engine) Distribution(req DistributionRequest) (*DistributionResponse, error) {
	c, err := e.column(req.Column, "column")
	if err != nil {
		return nil, err
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}
	var split *grouping
	if req.SplitBy != "" {
		sc, err := e.column(req.SplitBy, "split column")
		if err != nil {
			return nil, err
		}
		if sc.Name == c.Name {
			return nil, invalidf("cannot split %s by itself", c.Name)
		}
		if split, err = e.groupingFor(sc); err != nil {
			return nil, err
		}
	}

	resp := &DistributionResponse{
		Column: c.Name,
		Label:  c.Label,
		Kind:   c.Kind,
		Cohort: e.DescribeFilters(req.Filters),
	}

	// bucket maps a row to a histogram bin or level index.
	var bucket func(i int) int
	var size int
	if c.Kind == dataset.KindContinuous {
		target := req.Bins
		if target <= 0 {
			target = 24
		}
		target = min(max(target, 5), 60)
		bins, index := niceBins(c.Stats.Min, c.Stats.Max, target, c.Decimals)
		resp.Mode = "histogram"
		resp.Bins = bins
		bucket = func(i int) int {
			v := c.Num(i)
			if math.IsNaN(v) {
				return -1
			}
			return index(v)
		}
		size = len(bins)
	} else {
		resp.Mode = "levels"
		resp.Levels = c.Levels
		bucket = c.Level
		size = len(c.Levels)
	}

	newGroup := func(key, label string) *DistGroup {
		return &DistGroup{Key: key, Label: label, Counts: make([]int, size)}
	}
	overall := newGroup("all", resp.Cohort)
	var groups []*DistGroup
	var groupVals [][]float64
	if split != nil {
		resp.SplitBy = split.col.Name
		resp.SplitByLabel = split.col.Label
		for _, l := range split.labels {
			groups = append(groups, newGroup(l, l))
		}
		groupVals = make([][]float64, len(groups))
	}
	var overallVals []float64

	bands := c.GeneralBands()
	bandCounts := make([]int, len(bands))

	for _, r := range rows {
		i := int(r)
		b := bucket(i)
		if b < 0 {
			continue
		}
		overall.Counts[b]++
		overall.N++
		v := c.Num(i)
		if c.IsNumeric() {
			overallVals = append(overallVals, v)
			for k, band := range bands {
				if band.Contains(v) {
					bandCounts[k]++
					break
				}
			}
		}
		if split != nil {
			if g := split.assign(i); g >= 0 {
				groups[g].Counts[b]++
				groups[g].N++
				if c.IsNumeric() {
					groupVals[g] = append(groupVals[g], v)
				}
			}
		}
	}

	finishGroup(overall, overallVals, c)
	resp.Overall = *overall
	resp.N = overall.N
	for k, g := range groups {
		finishGroup(g, groupVals[k], c)
		resp.Groups = append(resp.Groups, *g)
	}
	if overall.N > 0 {
		for k, band := range bands {
			resp.Bands = append(resp.Bands, BandShare{
				Label: band.Label, Tone: band.Tone, Min: band.Min, Max: band.Max,
				Count: bandCounts[k], Share: float64(bandCounts[k]) / float64(overall.N),
			})
		}
	}
	return resp, nil
}

func finishGroup(g *DistGroup, vals []float64, c *dataset.Column) {
	g.Shares = make([]float64, len(g.Counts))
	if g.N > 0 {
		for i, n := range g.Counts {
			g.Shares[i] = float64(n) / float64(g.N)
		}
	}
	g.LowN = g.N < MinGroupSize
	if c.IsNumeric() && c.Kind != dataset.KindBinary {
		g.Summary = summarize(vals)
	}
}

// niceBins covers [lo, hi] with about target bins of a round width and
// returns them with a function mapping a value to its bin.
func niceBins(lo, hi float64, target, decimals int) ([]Bin, func(float64) int) {
	if !(hi > lo) {
		hi = lo + 1
	}
	raw := (hi - lo) / float64(target)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	step := 10 * mag
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			step = m * mag
			break
		}
	}
	if decimals == 0 && step < 1 {
		step = 1
	}
	start := math.Floor(lo/step) * step
	n := int(math.Ceil((hi - start) / step))
	if n < 1 {
		n = 1
	}
	// Values equal to the top edge fall in the last bin.
	if start+float64(n)*step <= hi {
		n++
	}
	bins := make([]Bin, n)
	for k := range bins {
		bins[k] = Bin{
			Start: roundTo(start+float64(k)*step, 10),
			End:   roundTo(start+float64(k+1)*step, 10),
		}
	}
	index := func(v float64) int {
		k := int(math.Floor((v - start) / step))
		return min(max(k, 0), n-1)
	}
	return bins, index
}
