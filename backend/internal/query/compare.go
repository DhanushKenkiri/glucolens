package query

import (
	"fmt"
	"math"

	"glucolens/internal/dataset"
)

// CompareRequest compares a metric across the groups of another column.
type CompareRequest struct {
	Metric  string   `json:"metric"`
	GroupBy string   `json:"groupBy"`
	Filters []Filter `json:"filters,omitempty"`
}

// CompareGroup is the metric within one group.
type CompareGroup struct {
	Key     string    `json:"key"`
	Label   string    `json:"label"`
	N       int       `json:"n"`
	Summary *Summary  `json:"summary,omitempty"`
	Rate    *Rate     `json:"rate,omitempty"`
	Shares  []float64 `json:"shares,omitempty"`
	LowN    bool      `json:"lowN,omitempty"`
}

// TestResult reports a significance test and effect size.
type TestResult struct {
	Name       string  `json:"name"`
	Statistic  float64 `json:"statistic"`
	DF         []int   `json:"df"`
	PValue     float64 `json:"pValue"`
	Effect     float64 `json:"effect"`
	EffectName string  `json:"effectName"`
	Strength   string  `json:"strength"`
}

// CompareResponse holds one entry per group plus the overall figure.
type CompareResponse struct {
	Metric       string         `json:"metric"`
	MetricLabel  string         `json:"metricLabel"`
	MetricKind   dataset.Kind   `json:"metricKind"`
	Mode         string         `json:"mode"` // numeric | rate | shares
	Levels       []string       `json:"levels,omitempty"`
	GroupBy      string         `json:"groupBy"`
	GroupByLabel string         `json:"groupByLabel"`
	Cohort       string         `json:"cohort"`
	N            int            `json:"n"`
	Overall      CompareGroup   `json:"overall"`
	Groups       []CompareGroup `json:"groups"`
	Test         TestResult     `json:"test"`
	Summary      string         `json:"summary"`
}

// Compare describes a metric within each group of another column.
func (e *Engine) Compare(req CompareRequest) (*CompareResponse, error) {
	m, err := e.column(req.Metric, "metric")
	if err != nil {
		return nil, err
	}
	gc, err := e.column(req.GroupBy, "group column")
	if err != nil {
		return nil, err
	}
	if m.Name == gc.Name {
		return nil, invalidf("cannot compare %s with itself", m.Name)
	}
	g, err := e.groupingFor(gc)
	if err != nil {
		return nil, err
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}

	resp := &CompareResponse{
		Metric: m.Name, MetricLabel: m.Label, MetricKind: m.Kind,
		GroupBy: gc.Name, GroupByLabel: gc.Label,
		Cohort: e.DescribeFilters(req.Filters),
	}
	k := len(g.labels)
	switch {
	case m.Kind == dataset.KindBinary:
		resp.Mode = "rate"
	case m.Kind == dataset.KindCategorical:
		resp.Mode = "shares"
		resp.Levels = m.Levels
	default:
		resp.Mode = "numeric"
	}

	vals := make([][]float64, k)
	var allVals []float64
	counts := make([][]float64, k) // group x metric level
	overallCounts := make([]float64, len(m.Levels))
	ns := make([]int, k)
	for i := range counts {
		counts[i] = make([]float64, len(m.Levels))
	}
	for _, r := range rows {
		i := int(r)
		grp := g.assign(i)
		if grp < 0 {
			continue
		}
		if resp.Mode == "numeric" {
			v := m.Num(i)
			if v != v { // NaN
				continue
			}
			vals[grp] = append(vals[grp], v)
			allVals = append(allVals, v)
			ns[grp]++
			continue
		}
		l := m.Level(i)
		if l < 0 {
			continue
		}
		counts[grp][l]++
		overallCounts[l]++
		ns[grp]++
	}

	total := 0
	for _, n := range ns {
		total += n
	}
	resp.N = total
	for idx, label := range g.labels {
		cg := CompareGroup{Key: label, Label: label, N: ns[idx], LowN: ns[idx] < MinGroupSize}
		switch resp.Mode {
		case "numeric":
			cg.Summary = summarize(append([]float64(nil), vals[idx]...))
		case "rate":
			r := newRate(int(counts[idx][1]), ns[idx])
			cg.Rate = &r
		case "shares":
			cg.Shares = shares(counts[idx], ns[idx])
		}
		resp.Groups = append(resp.Groups, cg)
	}
	overall := CompareGroup{Key: "all", Label: "Everyone in view", N: total}
	switch resp.Mode {
	case "numeric":
		overall.Summary = summarize(allVals)
		f, p, eta, d1, d2 := anova(vals)
		resp.Test = TestResult{
			Name: "One-way ANOVA", Statistic: round(finite(f), 3), DF: []int{d1, d2},
			PValue: p, Effect: round(eta, 4), EffectName: "η (correlation ratio)",
			Strength: strengthOf(eta, p),
		}
	default:
		if resp.Mode == "rate" {
			r := newRate(int(overallCounts[1]), total)
			overall.Rate = &r
		} else {
			overall.Shares = shares(overallCounts, total)
		}
		chi2, df, n := chiSquare(counts)
		p := chiSquarePValue(chi2, df)
		v := cramersV(chi2, n, counts)
		resp.Test = TestResult{
			Name: "Chi-square test", Statistic: round(chi2, 3), DF: []int{df},
			PValue: p, Effect: round(v, 4), EffectName: "Cramér's V",
			Strength: strengthOf(v, p),
		}
	}
	resp.Overall = overall
	resp.Summary = compareSummary(resp, m, g)
	return resp, nil
}

func shares(counts []float64, n int) []float64 {
	out := make([]float64, len(counts))
	if n == 0 {
		return out
	}
	for i, c := range counts {
		out[i] = c / float64(n)
	}
	return out
}

// cramersV computes Cramér's V for a contingency table, counting only the
// rows and columns that have data.
func cramersV(chi2, n float64, table [][]float64) float64 {
	if n == 0 || len(table) == 0 {
		return 0
	}
	rows, cols := 0, 0
	colUsed := make([]bool, len(table[0]))
	for _, r := range table {
		sum := 0.0
		for j, v := range r {
			sum += v
			if v > 0 {
				colUsed[j] = true
			}
		}
		if sum > 0 {
			rows++
		}
	}
	for _, u := range colUsed {
		if u {
			cols++
		}
	}
	k := min(rows, cols) - 1
	if k <= 0 {
		return 0
	}
	v := chi2 / (n * float64(k))
	if v <= 0 {
		return 0
	}
	if v > 1 {
		v = 1
	}
	return math.Sqrt(v)
}

func compareSummary(resp *CompareResponse, m *dataset.Column, g *grouping) string {
	hi, lo := -1, -1
	var hiV, loV float64
	for i, g := range resp.Groups {
		if g.N < MinGroupSize {
			continue
		}
		var v float64
		switch {
		case g.Summary != nil:
			v = g.Summary.Median
			if m.Kind == dataset.KindDiscrete {
				v = g.Summary.Mean
			}
		case g.Rate != nil:
			v = g.Rate.Value
		default:
			continue
		}
		if hi < 0 || v > hiV {
			hi, hiV = i, v
		}
		if lo < 0 || v < loV {
			lo, loV = i, v
		}
	}
	if hi < 0 || hi == lo {
		return fmt.Sprintf("Not enough people in each %s group to compare.", lowerFirst(resp.GroupByLabel))
	}
	format := func(v float64) string {
		if resp.Mode == "rate" {
			return pct(v)
		}
		return m.FormatValue(v)
	}
	stat := "the median " + m.Label
	switch {
	case resp.Mode == "rate":
		stat = "the share with " + lowerFirst(m.Label)
	case m.Kind == dataset.KindDiscrete:
		stat = "the average " + m.Label
	}
	strength := resp.Test.Strength
	if strength == StrengthNone {
		return fmt.Sprintf("No meaningful difference in %s across %s groups (%s to %s).",
			stat, lowerFirst(resp.GroupByLabel), format(loV), format(hiV))
	}
	if g.col.Kind == dataset.KindBinary {
		word := "higher"
		if hi == 0 {
			word = "lower"
		}
		return fmt.Sprintf("%s is %s %s (%s) than without (%s): a %s difference.",
			upperFirst(stat), word, g.phrase(1), format(resp.Groups[1].numeric()), format(resp.Groups[0].numeric()), strength)
	}
	return fmt.Sprintf("%s is highest in the %s group (%s) and lowest in the %s group (%s): a %s difference.",
		upperFirst(stat), g.phrase(hi), format(hiV), g.phrase(lo), format(loV), strength)
}

// numeric returns the figure compared in summaries: the rate, or the median
// (the mean for discrete counts such as drinks per week).
func (g CompareGroup) numeric() float64 {
	switch {
	case g.Rate != nil:
		return g.Rate.Value
	case g.Summary != nil:
		return g.Summary.Median
	}
	return 0
}
