package query

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"glucolens/internal/dataset"
)

// Outcome names the event being analysed: a preset id, or a column with the
// levels that count as the event, or a numeric threshold (both ends
// inclusive). A yes/no column with nothing else means "yes".
type Outcome struct {
	Preset string   `json:"preset,omitempty"`
	Column string   `json:"column,omitempty"`
	Equals []string `json:"equals,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// OutcomeInfo describes a resolved outcome for display.
type OutcomeInfo struct {
	ID         string `json:"id,omitempty"`
	Label      string `json:"label"`
	Column     string `json:"column"`
	Definition string `json:"definition"`
}

type compiledOutcome struct {
	info    OutcomeInfo
	def     dataset.OutcomeDef
	col     *dataset.Column
	allowed []bool
	lo, hi  float64
	ranged  bool
}

// event reports whether row i is an event and whether its value is known.
func (o *compiledOutcome) event(i int) (bool, bool) {
	if o.ranged {
		v := o.col.Num(i)
		if math.IsNaN(v) {
			return false, false
		}
		return v >= o.lo && v <= o.hi, true
	}
	l := o.col.Level(i)
	if l < 0 {
		return false, false
	}
	return o.allowed[l], true
}

func (e *Engine) compileOutcome(o Outcome) (*compiledOutcome, error) {
	var def dataset.OutcomeDef
	info := OutcomeInfo{}
	if o.Preset != "" {
		p, ok := e.ds.OutcomePreset(o.Preset)
		if !ok {
			ids := make([]string, len(e.ds.Outcomes))
			for i, p := range e.ds.Outcomes {
				ids[i] = p.ID
			}
			return nil, invalidf("unknown outcome preset %q (presets: %s)", o.Preset, strings.Join(ids, ", "))
		}
		def = p.Outcome
		info.ID, info.Label = p.ID, p.Label
	} else {
		if o.Column == "" {
			return nil, invalidf("outcome needs a preset or a column")
		}
		def = dataset.OutcomeDef{Column: o.Column, Equals: o.Equals, Min: o.Min, Max: o.Max}
	}
	if err := e.ds.ValidateOutcome(def); err != nil {
		return nil, invalidf("%s", err.Error())
	}
	c, _ := e.ds.Column(def.Column)
	def = canonicalOutcome(c, def)
	if info.ID == "" {
		// A custom definition that matches a preset is reported as that preset.
		for _, p := range e.ds.Outcomes {
			if pc, ok := e.ds.Column(p.Outcome.Column); ok && sameOutcome(canonicalOutcome(pc, p.Outcome), def) {
				info.ID, info.Label = p.ID, p.Label
				break
			}
		}
	}
	co := &compiledOutcome{def: def, col: c}
	switch {
	case len(def.Equals) > 0:
		co.allowed = make([]bool, len(c.Levels))
		var names []string
		for _, v := range def.Equals {
			idx, _ := c.FindLevel(v)
			co.allowed[idx] = true
			names = append(names, c.Levels[idx])
		}
		info.Definition = c.Label + ": " + strings.Join(names, " or ")
	case def.Min != nil || def.Max != nil:
		co.ranged = true
		co.lo, co.hi = math.Inf(-1), math.Inf(1)
		if def.Min != nil {
			co.lo = *def.Min
		}
		if def.Max != nil {
			co.hi = *def.Max
		}
		info.Definition = c.Label + " " + describeRange(c, def.Min, def.Max)
	default: // yes/no column: "yes" is the event
		co.allowed = []bool{false, true}
		info.Definition = c.Label + ": " + c.Levels[1]
	}
	info.Column = c.Name
	if info.Label == "" {
		info.Label = info.Definition
	}
	co.info = info
	return co, nil
}

// canonicalOutcome spells levels exactly as the column does and makes the
// implicit "yes" of a bare yes/no column explicit.
func canonicalOutcome(c *dataset.Column, d dataset.OutcomeDef) dataset.OutcomeDef {
	if len(d.Equals) == 0 && d.Min == nil && d.Max == nil && c.Kind == dataset.KindBinary {
		d.Equals = []string{c.Levels[1]}
		return d
	}
	if len(d.Equals) > 0 {
		eq := make([]string, 0, len(d.Equals))
		for _, v := range d.Equals {
			if idx, ok := c.FindLevel(v); ok {
				eq = append(eq, c.Levels[idx])
			}
		}
		sort.Strings(eq)
		d.Equals = eq
	}
	return d
}

func sameOutcome(a, b dataset.OutcomeDef) bool {
	if a.Column != b.Column || len(a.Equals) != len(b.Equals) {
		return false
	}
	for i := range a.Equals {
		if !strings.EqualFold(a.Equals[i], b.Equals[i]) {
			return false
		}
	}
	eq := func(x, y *float64) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	return eq(a.Min, b.Min) && eq(a.Max, b.Max)
}

// RiskLevel is the outcome rate within one group of the factor.
type RiskLevel struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	Rate         Rate    `json:"rate"`
	RelativeRisk *Ratio  `json:"relativeRisk,omitempty"`
	Difference   float64 `json:"difference"`
	IsReference  bool    `json:"isReference,omitempty"`
	LowN         bool    `json:"lowN,omitempty"`
}

// Association summarises how strongly the factor and outcome are linked.
type Association struct {
	ChiSquare float64 `json:"chiSquare"`
	DF        int     `json:"df"`
	PValue    float64 `json:"pValue"`
	CramersV  float64 `json:"cramersV"`
	Spread    float64 `json:"spread"`
	Strength  string  `json:"strength"`
}

// Stratum repeats the analysis within one group of a third column.
type Stratum struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	N           int         `json:"n"`
	Base        Rate        `json:"base"`
	Levels      []RiskLevel `json:"levels"`
	Association Association `json:"association"`
}

// RiskRequest asks how an outcome's rate varies across a factor's groups.
type RiskRequest struct {
	Outcome    Outcome  `json:"outcome"`
	Factor     string   `json:"factor"`
	Reference  string   `json:"reference,omitempty"`
	Filters    []Filter `json:"filters,omitempty"`
	StratifyBy string   `json:"stratifyBy,omitempty"`
}

// RiskResponse holds the per-group rates and the overall verdict.
type RiskResponse struct {
	Outcome         OutcomeInfo `json:"outcome"`
	Factor          string      `json:"factor"`
	FactorLabel     string      `json:"factorLabel"`
	Grouping        string      `json:"grouping"`
	Reference       string      `json:"reference"`
	Cohort          string      `json:"cohort"`
	N               int         `json:"n"`
	Base            Rate        `json:"base"`
	Levels          []RiskLevel `json:"levels"`
	Association     Association `json:"association"`
	Verdict         string      `json:"verdict"`
	StratifyBy      string      `json:"stratifyBy,omitempty"`
	StratifyByLabel string      `json:"stratifyByLabel,omitempty"`
	Strata          []Stratum   `json:"strata,omitempty"`
}

// Risk computes the outcome rate in each group of the factor, relative risk
// against a reference group, and a test of association.
func (e *Engine) Risk(req RiskRequest) (*RiskResponse, error) {
	out, err := e.compileOutcome(req.Outcome)
	if err != nil {
		return nil, err
	}
	fc, err := e.column(req.Factor, "factor")
	if err != nil {
		return nil, err
	}
	if fc.Name == out.col.Name {
		return nil, invalidf("the factor cannot be the outcome column itself")
	}
	g, err := e.groupingFor(fc)
	if err != nil {
		return nil, err
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}

	levels, assoc, base, ref, err := e.riskTable(rows, out, g, req.Reference)
	if err != nil {
		return nil, err
	}
	resp := &RiskResponse{
		Outcome:     out.info,
		Factor:      fc.Name,
		FactorLabel: fc.Label,
		Grouping:    g.source,
		Reference:   g.labels[ref],
		Cohort:      e.DescribeFilters(req.Filters),
		N:           base.N,
		Base:        base,
		Levels:      levels,
		Association: assoc,
	}
	resp.Verdict = riskVerdict(out.info.Label, g, levels, assoc)

	if req.StratifyBy != "" {
		sc, err := e.column(req.StratifyBy, "stratify column")
		if err != nil {
			return nil, err
		}
		if sc.Name == fc.Name || sc.Name == out.col.Name {
			return nil, invalidf("stratify by a third column, not the factor or the outcome")
		}
		sg, err := e.groupingFor(sc)
		if err != nil {
			return nil, err
		}
		resp.StratifyBy, resp.StratifyByLabel = sc.Name, sc.Label
		buckets := make([][]int32, len(sg.labels))
		for _, r := range rows {
			if s := sg.assign(int(r)); s >= 0 {
				buckets[s] = append(buckets[s], r)
			}
		}
		for s, label := range sg.labels {
			lv, as, b, _, err := e.riskTable(buckets[s], out, g, resp.Reference)
			if err != nil {
				return nil, err
			}
			resp.Strata = append(resp.Strata, Stratum{
				Key: label, Label: label, N: b.N, Base: b, Levels: lv, Association: as,
			})
		}
	}
	return resp, nil
}

// riskTable counts events per group and derives rates, relative risks and
// the association statistics.
func (e *Engine) riskTable(rows []int32, out *compiledOutcome, g *grouping, reference string) ([]RiskLevel, Association, Rate, int, error) {
	k := len(g.labels)
	events := make([]int, k)
	ns := make([]int, k)
	totalEvents, total := 0, 0
	for _, r := range rows {
		i := int(r)
		grp := g.assign(i)
		if grp < 0 {
			continue
		}
		ev, known := out.event(i)
		if !known {
			continue
		}
		ns[grp]++
		total++
		if ev {
			events[grp]++
			totalEvents++
		}
	}
	ref, err := g.referenceIndex(reference, ns)
	if err != nil {
		return nil, Association{}, Rate{}, 0, err
	}

	levels := make([]RiskLevel, k)
	table := make([][]float64, k)
	refRate := newRate(events[ref], ns[ref])
	for i, label := range g.labels {
		rate := newRate(events[i], ns[i])
		lv := RiskLevel{
			Key: label, Label: label, Rate: rate,
			IsReference: i == ref,
			LowN:        ns[i] < MinGroupSize,
		}
		if ns[i] > 0 && ns[ref] > 0 {
			lv.Difference = rate.Value - refRate.Value
		}
		if i != ref {
			lv.RelativeRisk = relativeRisk(events[i], ns[i], events[ref], ns[ref])
		}
		levels[i] = lv
		table[i] = []float64{float64(events[i]), float64(ns[i] - events[i])}
	}

	chi2, df, n := chiSquare(table)
	p := chiSquarePValue(chi2, df)
	v := cramersV(chi2, n, table)
	spread := 0.0
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, lv := range levels {
		if lv.Rate.N >= MinGroupSize {
			lo = math.Min(lo, lv.Rate.Value)
			hi = math.Max(hi, lv.Rate.Value)
		}
	}
	if hi >= lo {
		spread = hi - lo
	}
	assoc := Association{
		ChiSquare: round(chi2, 3), DF: df, PValue: p,
		CramersV: round(v, 4), Spread: spread, Strength: strengthOf(v, p),
	}
	return levels, assoc, newRate(totalEvents, total), ref, nil
}

func riskVerdict(outcome string, g *grouping, levels []RiskLevel, a Association) string {
	factor := g.col.Label
	hi, lo := -1, -1
	for i, lv := range levels {
		if lv.Rate.N < MinGroupSize {
			continue
		}
		if hi < 0 || lv.Rate.Value > levels[hi].Rate.Value {
			hi = i
		}
		if lo < 0 || lv.Rate.Value < levels[lo].Rate.Value {
			lo = i
		}
	}
	if hi < 0 || hi == lo {
		return fmt.Sprintf("Not enough people in each %s group to compare.", lowerFirst(factor))
	}
	o := lowerFirst(outcome)
	if a.Strength == StrengthNone {
		return fmt.Sprintf("No meaningful difference: %s ranges only from %s to %s across %s groups.",
			o, pct(levels[lo].Rate.Value), pct(levels[hi].Rate.Value), lowerFirst(factor))
	}
	strength := strings.ToUpper(a.Strength[:1]) + a.Strength[1:]
	subject := strings.ToUpper(o[:1]) + o[1:]
	if g.col.Kind == dataset.KindBinary {
		more, than := "more", "than"
		if hi == 0 { // the "no" group has the higher rate
			more = "less"
		}
		return fmt.Sprintf("%s link. %s is %s common %s (%s) %s without (%s).",
			strength, subject, more, g.phrase(1), pct(levels[1].Rate.Value), than, pct(levels[0].Rate.Value))
	}
	return fmt.Sprintf("%s link. %s is most common in the %s group (%s) and least common in the %s group (%s).",
		strength, subject, g.phrase(hi), pct(levels[hi].Rate.Value), g.phrase(lo), pct(levels[lo].Rate.Value))
}

// RankRequest ranks every candidate factor by its link to the outcome.
type RankRequest struct {
	Outcome        Outcome  `json:"outcome"`
	Filters        []Filter `json:"filters,omitempty"`
	IncludeMarkers bool     `json:"includeMarkers,omitempty"`
}

// RankLevel is a compact per-group rate for ranking views.
type RankLevel struct {
	Label string  `json:"label"`
	Rate  float64 `json:"rate"`
	N     int     `json:"n"`
	LowN  bool    `json:"lowN,omitempty"`
}

// RankedFactor is one factor in the ranking.
type RankedFactor struct {
	Column      string      `json:"column"`
	Label       string      `json:"label"`
	Section     string      `json:"section"`
	Role        string      `json:"role"`
	Association Association `json:"association"`
	Levels      []RankLevel `json:"levels"`
	Highest     RankLevel   `json:"highest"`
	Lowest      RankLevel   `json:"lowest"`
	Verdict     string      `json:"verdict"`
}

// RankResponse lists factors from the strongest link to the weakest.
type RankResponse struct {
	Outcome OutcomeInfo    `json:"outcome"`
	Cohort  string         `json:"cohort"`
	N       int            `json:"n"`
	Base    Rate           `json:"base"`
	Factors []RankedFactor `json:"factors"`
	Skipped []string       `json:"skipped,omitempty"`
}

// Rank scores each candidate factor against the outcome by Cramér's V.
func (e *Engine) Rank(req RankRequest) (*RankResponse, error) {
	out, err := e.compileOutcome(req.Outcome)
	if err != nil {
		return nil, err
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}
	resp := &RankResponse{Outcome: out.info, Cohort: e.DescribeFilters(req.Filters)}

	for _, c := range e.ds.Columns {
		if !c.Analysable() || c.Name == out.col.Name || c.Role == dataset.RoleOutcome {
			continue
		}
		if c.Role == dataset.RoleMarker && !req.IncludeMarkers {
			continue
		}
		g, err := e.groupingFor(c)
		if err != nil {
			resp.Skipped = append(resp.Skipped, c.Name)
			continue
		}
		levels, assoc, base, _, err := e.riskTable(rows, out, g, "")
		if err != nil {
			return nil, err
		}
		if resp.N == 0 {
			resp.N, resp.Base = base.N, base
		}
		rf := RankedFactor{
			Column: c.Name, Label: c.Label, Section: c.Section, Role: c.Role,
			Association: assoc,
			Verdict:     riskVerdict(out.info.Label, g, levels, assoc),
		}
		first := true
		for _, lv := range levels {
			rl := RankLevel{Label: lv.Label, Rate: lv.Rate.Value, N: lv.Rate.N, LowN: lv.LowN}
			rf.Levels = append(rf.Levels, rl)
			if lv.LowN {
				continue
			}
			if first || rl.Rate > rf.Highest.Rate {
				rf.Highest = rl
			}
			if first || rl.Rate < rf.Lowest.Rate {
				rf.Lowest = rl
			}
			first = false
		}
		resp.Factors = append(resp.Factors, rf)
	}
	sort.SliceStable(resp.Factors, func(i, j int) bool {
		return resp.Factors[i].Association.CramersV > resp.Factors[j].Association.CramersV
	})
	return resp, nil
}
