package assistant

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"

	"glucolens/internal/dataset"
	"glucolens/internal/query"
)

// stateKeyPatient is the session-state key holding the user's own values.
const stateKeyPatient = "patient"

// Tool names, shared by the agent, the event stream and the UI.
const (
	toolDistribution = "get_distribution"
	toolCompare      = "compare_groups"
	toolRisk         = "analyze_risk"
	toolRank         = "rank_risk_factors"
	toolPercentile   = "get_percentile"
	toolSimilar      = "find_similar_people"
)

// DistributionArgs are the inputs of get_distribution.
type DistributionArgs struct {
	Column  string   `json:"column" jsonschema:"Column to describe, e.g. hba1c or smoking_status"`
	SplitBy string   `json:"split_by,omitempty" jsonschema:"Optional column to split the results by, e.g. gender"`
	Filters []string `json:"filters,omitempty" jsonschema:"Optional filters as short text, one condition each, for example: 'gender = Female', 'age >= 50', 'age < 65', 'smoking_status in Never, Former', 'family_history_diabetes = Yes'"`
}

// CompareArgs are the inputs of compare_groups.
type CompareArgs struct {
	Metric  string   `json:"metric" jsonschema:"Column to compare: a number (e.g. hba1c), a yes/no column or a category"`
	GroupBy string   `json:"group_by" jsonschema:"Column that defines the groups, e.g. family_history_diabetes or age"`
	Filters []string `json:"filters,omitempty" jsonschema:"Optional filters as short text, one condition each, for example: 'gender = Female', 'age >= 50', 'age < 65', 'smoking_status in Never, Former', 'family_history_diabetes = Yes'"`
}

// OutcomeArgs describe the outcome for the risk tools.
type OutcomeArgs struct {
	Outcome       string   `json:"outcome" jsonschema:"An outcome preset id (see instructions) or a column name"`
	OutcomeValues []string `json:"outcome_values,omitempty" jsonschema:"For a category column: the values that count as the outcome, e.g. [\"Type 2\"]"`
	OutcomeMin    *float64 `json:"outcome_min,omitempty" jsonschema:"For a numeric column: the outcome is a value of at least this, e.g. 6.5 for hba1c"`
	OutcomeMax    *float64 `json:"outcome_max,omitempty" jsonschema:"For a numeric column: the outcome is a value of at most this"`
}

// RiskArgs are the inputs of analyze_risk.
type RiskArgs struct {
	OutcomeArgs
	Factor     string   `json:"factor" jsonschema:"Column whose groups are compared, e.g. smoking_status or bmi"`
	Reference  string   `json:"reference,omitempty" jsonschema:"Optional group to compare the others against, e.g. Never"`
	StratifyBy string   `json:"stratify_by,omitempty" jsonschema:"Optional third column (e.g. age) to repeat the analysis within each of its groups, to check for confounding"`
	Filters    []string `json:"filters,omitempty" jsonschema:"Optional filters as short text, one condition each, for example: 'gender = Female', 'age >= 50', 'age < 65', 'smoking_status in Never, Former', 'family_history_diabetes = Yes'"`
}

// RankArgs are the inputs of rank_risk_factors.
type RankArgs struct {
	OutcomeArgs
	IncludeLabTests bool     `json:"include_lab_tests,omitempty" jsonschema:"Also rank glucose, HbA1c and insulin measurements (default false)"`
	Filters         []string `json:"filters,omitempty" jsonschema:"Optional filters as short text, one condition each, for example: 'gender = Female', 'age >= 50', 'age < 65', 'smoking_status in Never, Former', 'family_history_diabetes = Yes'"`
}

// PercentileArgs are the inputs of get_percentile.
type PercentileArgs struct {
	Columns []string `json:"columns,omitempty" jsonschema:"Columns to look up; defaults to all of the user's shared values"`
	Value   *float64 `json:"value,omitempty" jsonschema:"Optional value to rank instead of the user's own (needs exactly one column)"`
	Filters []string `json:"filters,omitempty" jsonschema:"Optional filters as short text, one condition each, for example: 'gender = Female', 'age >= 50', 'age < 65', 'smoking_status in Never, Former', 'family_history_diabetes = Yes'"`
}

// SimilarArgs are the inputs of find_similar_people.
type SimilarArgs struct {
	MatchOn string `json:"match_on,omitempty" jsonschema:"all (default): match on every shared value; lifestyle: match on background, lifestyle, history and body only"`
	K       int    `json:"k,omitempty" jsonschema:"How many similar people to use (default 500)"`
}

// View tells the UI which query reproduces a tool result as a chart.
type View struct {
	Kind    string `json:"kind"`
	Request any    `json:"request"`
}

func (o OutcomeArgs) toOutcome(ds *dataset.Dataset) query.Outcome {
	name := strings.TrimSpace(o.Outcome)
	if _, ok := ds.OutcomePreset(name); ok && len(o.OutcomeValues) == 0 && o.OutcomeMin == nil && o.OutcomeMax == nil {
		return query.Outcome{Preset: name}
	}
	return query.Outcome{Column: name, Equals: o.OutcomeValues, Min: o.OutcomeMin, Max: o.OutcomeMax}
}

// Request builders, shared by the tool handlers and the UI views.

func distributionRequest(ds *dataset.Dataset, a DistributionArgs) (query.DistributionRequest, error) {
	f, err := parseFilters(ds, a.Filters)
	return query.DistributionRequest{Column: a.Column, SplitBy: a.SplitBy, Filters: f}, err
}

func compareRequest(ds *dataset.Dataset, a CompareArgs) (query.CompareRequest, error) {
	f, err := parseFilters(ds, a.Filters)
	return query.CompareRequest{Metric: a.Metric, GroupBy: a.GroupBy, Filters: f}, err
}

func riskRequest(ds *dataset.Dataset, a RiskArgs) (query.RiskRequest, error) {
	f, err := parseFilters(ds, a.Filters)
	return query.RiskRequest{
		Outcome: a.toOutcome(ds), Factor: a.Factor, Reference: a.Reference,
		StratifyBy: a.StratifyBy, Filters: f,
	}, err
}

func rankRequest(ds *dataset.Dataset, a RankArgs) (query.RankRequest, error) {
	f, err := parseFilters(ds, a.Filters)
	return query.RankRequest{Outcome: a.toOutcome(ds), IncludeMarkers: a.IncludeLabTests, Filters: f}, err
}

var errNoValues = errors.New("the user has not shared their own values, so there is nothing to compare; " +
	"ask them to add their checkup values under My values (or give an explicit value)")

func profileRequest(ds *dataset.Dataset, a PercentileArgs, patient map[string]any) (query.ProfileRequest, error) {
	filters, err := parseFilters(ds, a.Filters)
	if err != nil {
		return query.ProfileRequest{}, err
	}
	values := map[string]any{}
	if a.Value != nil {
		if len(a.Columns) != 1 {
			return query.ProfileRequest{}, errors.New("give exactly one column together with value")
		}
		values[a.Columns[0]] = *a.Value
		return query.ProfileRequest{Values: values, Filters: filters}, nil
	}
	if len(patient) == 0 {
		return query.ProfileRequest{}, errNoValues
	}
	for k, v := range patient {
		values[k] = v
	}
	return query.ProfileRequest{Values: values, Columns: a.Columns, Filters: filters}, nil
}

func similarRequest(a SimilarArgs, patient map[string]any) (query.SimilarRequest, error) {
	if len(patient) == 0 {
		return query.SimilarRequest{}, errNoValues
	}
	values := map[string]any{}
	for k, v := range patient {
		values[k] = v
	}
	return query.SimilarRequest{Values: values, MatchOn: a.MatchOn, K: a.K}, nil
}

// patientFromState reads the user's values stored for this session.
func patientFromState(st session.ReadonlyState) map[string]any {
	if st == nil {
		return nil
	}
	v, err := st.Get(stateKeyPatient)
	if err != nil {
		return nil
	}
	m, _ := v.(map[string]any)
	return m
}

// buildTools creates the six dataset tools. None of them can read files,
// run code or reach the network: they only call the query engine.
func buildTools(e *query.Engine) ([]tool.Tool, error) {
	ds := e.Dataset()
	var tools []tool.Tool
	add := func(t tool.Tool, err error) error {
		if err != nil {
			return err
		}
		tools = append(tools, t)
		return nil
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolDistribution,
		Description: "Describe how one column's values are spread among people in the dataset: summary statistics and clinical-band shares for numbers, or the share of each value for categories. Optionally split by another column and filter the people.",
	}, func(_ agent.Context, a DistributionArgs) (map[string]any, error) {
		req, err := distributionRequest(ds, a)
		if err != nil {
			return nil, err
		}
		res, err := e.Distribution(req)
		if err != nil {
			return nil, err
		}
		return compactDistribution(ds, res), nil
	})); err != nil {
		return nil, err
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolCompare,
		Description: "Compare a column across the groups of another column (for example HbA1c by family history, or diagnosed diabetes by age group), with a significance test.",
	}, func(_ agent.Context, a CompareArgs) (map[string]any, error) {
		req, err := compareRequest(ds, a)
		if err != nil {
			return nil, err
		}
		res, err := e.Compare(req)
		if err != nil {
			return nil, err
		}
		return compactCompare(ds, res), nil
	})); err != nil {
		return nil, err
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolRisk,
		Description: "Risk analysis: the rate of an outcome (for example diagnosed diabetes) in each group of a factor (for example smoking status), with 95% confidence intervals, relative risk against a reference group and a verdict on how strong the link is. Can stratify by a third column to check confounding.",
	}, func(_ agent.Context, a RiskArgs) (map[string]any, error) {
		req, err := riskRequest(ds, a)
		if err != nil {
			return nil, err
		}
		res, err := e.Risk(req)
		if err != nil {
			return nil, err
		}
		return compactRisk(res), nil
	})); err != nil {
		return nil, err
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolRank,
		Description: "Rank every background, lifestyle, history and body factor by how strongly it is linked to an outcome, strongest first.",
	}, func(_ agent.Context, a RankArgs) (map[string]any, error) {
		req, err := rankRequest(ds, a)
		if err != nil {
			return nil, err
		}
		res, err := e.Rank(req)
		if err != nil {
			return nil, err
		}
		return compactRank(res), nil
	})); err != nil {
		return nil, err
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolPercentile,
		Description: "Place the user's own shared values (or one explicit value) among people in the dataset: percentile, typical range and clinical band for each value.",
	}, func(ctx agent.Context, a PercentileArgs) (map[string]any, error) {
		req, err := profileRequest(ds, a, patientFromState(ctx.ReadonlyState()))
		if err != nil {
			return nil, err
		}
		res, err := e.Profile(req)
		if err != nil {
			return nil, err
		}
		return compactProfile(res), nil
	})); err != nil {
		return nil, err
	}

	if err := add(functiontool.New(functiontool.Config{
		Name:        toolSimilar,
		Description: "Find the people in the dataset most similar to the user (from their shared values) and report how common each outcome is among them compared with everyone.",
	}, func(ctx agent.Context, a SimilarArgs) (map[string]any, error) {
		req, err := similarRequest(a, patientFromState(ctx.ReadonlyState()))
		if err != nil {
			return nil, err
		}
		res, err := e.Similar(req)
		if err != nil {
			return nil, err
		}
		return compactSimilar(ds, res), nil
	})); err != nil {
		return nil, err
	}
	return tools, nil
}

// decodeArgs converts raw tool-call arguments into a typed struct.
func decodeArgs(args map[string]any, out any) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// viewFor rebuilds the query a tool call ran, so the UI can draw it with
// the same chart components the other tabs use.
func viewFor(ds *dataset.Dataset, name string, args map[string]any, patient map[string]any) *View {
	switch name {
	case toolDistribution:
		var a DistributionArgs
		if decodeArgs(args, &a) == nil {
			if req, err := distributionRequest(ds, a); err == nil {
				return &View{Kind: "distribution", Request: req}
			}
		}
	case toolCompare:
		var a CompareArgs
		if decodeArgs(args, &a) == nil {
			if req, err := compareRequest(ds, a); err == nil {
				return &View{Kind: "compare", Request: req}
			}
		}
	case toolRisk:
		var a RiskArgs
		if decodeArgs(args, &a) == nil {
			if req, err := riskRequest(ds, a); err == nil {
				return &View{Kind: "risk", Request: req}
			}
		}
	case toolRank:
		var a RankArgs
		if decodeArgs(args, &a) == nil {
			if req, err := rankRequest(ds, a); err == nil {
				return &View{Kind: "rank", Request: req}
			}
		}
	case toolPercentile:
		var a PercentileArgs
		if decodeArgs(args, &a) == nil {
			if req, err := profileRequest(ds, a, patient); err == nil {
				return &View{Kind: "profile", Request: req}
			}
		}
	case toolSimilar:
		var a SimilarArgs
		if decodeArgs(args, &a) == nil {
			if req, err := similarRequest(a, patient); err == nil {
				return &View{Kind: "similar", Request: req}
			}
		}
	}
	return nil
}

// labelFor describes a tool call in a few words for the progress line.
func labelFor(ds *dataset.Dataset, name string, args map[string]any) string {
	col := func(key string) string {
		if s, ok := args[key].(string); ok {
			if c, ok := ds.Column(s); ok {
				return lower(c.Label)
			}
			if p, ok := ds.OutcomePreset(s); ok {
				return lower(p.Label)
			}
			return strings.ReplaceAll(s, "_", " ")
		}
		return ""
	}
	switch name {
	case toolDistribution:
		if by := col("split_by"); by != "" {
			return fmt.Sprintf("Looking at %s by %s", col("column"), by)
		}
		return "Looking at the spread of " + col("column")
	case toolCompare:
		return fmt.Sprintf("Comparing %s by %s", col("metric"), col("group_by"))
	case toolRisk:
		s := fmt.Sprintf("Checking %s rates by %s", col("outcome"), col("factor"))
		if by := col("stratify_by"); by != "" {
			s += ", within " + by + " groups"
		}
		return s
	case toolRank:
		return "Ranking factors linked to " + col("outcome")
	case toolPercentile:
		return "Placing your values among everyone"
	case toolSimilar:
		return "Finding people similar to you"
	}
	return "Running " + strings.ReplaceAll(name, "_", " ")
}

func lower(s string) string { return dataset.LowerFirst(s) }

// Compact results: small, rounded and labelled, to keep model prompts short.

func pctStr(v float64) string { return fmt.Sprintf("%.1f%%", v*100) }

func num(c *dataset.Column, v float64) string { return c.FormatValue(v) }

func compactDistribution(ds *dataset.Dataset, r *query.DistributionResponse) map[string]any {
	c, _ := ds.Column(r.Column)
	out := map[string]any{"column": r.Column, "label": r.Label, "people": r.N, "group": r.Cohort}
	if c.Unit != "" {
		out["unit"] = c.Unit
	}
	if s := r.Overall.Summary; s != nil {
		out["summary"] = map[string]any{
			"mean": num(c, s.Mean), "median": num(c, s.Median), "p25": num(c, s.P25), "p75": num(c, s.P75),
			"p5": num(c, s.P5), "p95": num(c, s.P95), "min": num(c, s.Min), "max": num(c, s.Max),
		}
	}
	if r.Mode == "levels" {
		var vals []map[string]any
		for i, l := range r.Levels {
			vals = append(vals, map[string]any{"value": l, "people": r.Overall.Counts[i], "share": pctStr(r.Overall.Shares[i])})
		}
		out["values"] = vals
	}
	if len(r.Bands) > 0 {
		var bands []map[string]any
		for _, b := range r.Bands {
			bands = append(bands, map[string]any{"band": b.Label, "share": pctStr(b.Share)})
		}
		out["clinical_bands"] = bands
	}
	if r.SplitBy != "" {
		out["split_by"] = r.SplitBy
		var groups []map[string]any
		for _, g := range r.Groups {
			gm := map[string]any{"group": g.Label, "people": g.N}
			if g.LowN {
				gm["too_few_people"] = true
			}
			if g.Summary != nil {
				gm["median"] = num(c, g.Summary.Median)
				gm["mean"] = num(c, g.Summary.Mean)
			} else if r.Mode == "levels" && g.N > 0 {
				shares := map[string]string{}
				for i, l := range r.Levels {
					shares[l] = pctStr(g.Shares[i])
				}
				gm["shares"] = shares
			}
			groups = append(groups, gm)
		}
		out["groups"] = groups
	}
	return out
}

func compactCompare(ds *dataset.Dataset, r *query.CompareResponse) map[string]any {
	m, _ := ds.Column(r.Metric)
	out := map[string]any{
		"metric": r.Metric, "group_by": r.GroupBy, "people": r.N, "group": r.Cohort,
		"summary": r.Summary,
		"test":    map[string]any{"name": r.Test.Name, "p_value": pValue(r.Test.PValue), "effect": r.Test.Effect, "strength": r.Test.Strength},
	}
	var groups []map[string]any
	for _, g := range r.Groups {
		gm := map[string]any{"group": g.Label, "people": g.N}
		if g.LowN {
			gm["too_few_people"] = true
		}
		switch {
		case g.Summary != nil:
			gm["median"] = num(m, g.Summary.Median)
			gm["mean"] = num(m, g.Summary.Mean)
			gm["middle_half"] = num(m, g.Summary.P25) + " to " + num(m, g.Summary.P75)
		case g.Rate != nil:
			gm["rate"] = pctStr(g.Rate.Value)
			gm["ci95"] = pctStr(g.Rate.Low) + " to " + pctStr(g.Rate.High)
		case g.Shares != nil:
			shares := map[string]string{}
			for i, l := range r.Levels {
				shares[l] = pctStr(g.Shares[i])
			}
			gm["shares"] = shares
		}
		groups = append(groups, gm)
	}
	out["groups"] = groups
	return out
}

func compactRisk(r *query.RiskResponse) map[string]any {
	levels := func(lvs []query.RiskLevel) []map[string]any {
		var out []map[string]any
		for _, lv := range lvs {
			m := map[string]any{"group": lv.Label, "people": lv.Rate.N, "rate": pctStr(lv.Rate.Value),
				"ci95": pctStr(lv.Rate.Low) + " to " + pctStr(lv.Rate.High)}
			if lv.IsReference {
				m["reference"] = true
			}
			if rr := lv.RelativeRisk; rr != nil {
				m["relative_risk"] = fmt.Sprintf("%.2f (%.2f to %.2f)", rr.Value, rr.Low, rr.High)
			}
			if lv.LowN {
				m["too_few_people"] = true
			}
			out = append(out, m)
		}
		return out
	}
	out := map[string]any{
		"outcome": r.Outcome.Label, "definition": r.Outcome.Definition, "factor": r.Factor,
		"people": r.N, "group": r.Cohort, "overall_rate": pctStr(r.Base.Value),
		"levels":   levels(r.Levels),
		"strength": r.Association.Strength, "p_value": pValue(r.Association.PValue),
		"cramers_v": r.Association.CramersV, "verdict": r.Verdict,
	}
	if r.StratifyBy != "" {
		var strata []map[string]any
		for _, s := range r.Strata {
			var rates []string
			for _, lv := range s.Levels {
				rates = append(rates, fmt.Sprintf("%s %s (n=%d)", lv.Label, pctStr(lv.Rate.Value), lv.Rate.N))
			}
			strata = append(strata, map[string]any{
				"stratum": s.Label, "people": s.N, "rates": strings.Join(rates, "; "), "strength": s.Association.Strength,
			})
		}
		out["stratified_by"] = r.StratifyBy
		out["strata"] = strata
	}
	return out
}

func compactRank(r *query.RankResponse) map[string]any {
	var top []map[string]any
	var none []string
	for _, f := range r.Factors {
		if f.Association.Strength == query.StrengthNone {
			none = append(none, f.Label)
			continue
		}
		if len(top) < 10 {
			top = append(top, map[string]any{
				"factor": f.Column, "label": f.Label, "strength": f.Association.Strength,
				"cramers_v": f.Association.CramersV,
				"highest":   fmt.Sprintf("%s: %s", f.Highest.Label, pctStr(f.Highest.Rate)),
				"lowest":    fmt.Sprintf("%s: %s", f.Lowest.Label, pctStr(f.Lowest.Rate)),
			})
		}
	}
	return map[string]any{
		"outcome": r.Outcome.Label, "people": r.N, "group": r.Cohort, "overall_rate": pctStr(r.Base.Value),
		"strongest_factors": top, "no_meaningful_link": none,
	}
}

func compactProfile(r *query.ProfileResponse) map[string]any {
	var items []map[string]any
	for _, it := range r.Items {
		m := map[string]any{"column": it.Column, "label": it.Label, "value": it.Display}
		if it.Percentile != nil {
			p := *it.Percentile
			m["percentile"] = p
			// Spelled out so the model cannot read a low percentile as "higher than most".
			m["meaning"] = fmt.Sprintf("higher than about %.0f%% of people and lower than about %.0f%%", p, 100-p)
		}
		if it.Median != nil {
			m["group_median"] = dataset.FormatNumber(*it.Median, 2)
		}
		if it.Position != "" {
			m["position"] = it.Position
		}
		if it.Band != nil {
			m["clinical_band"] = it.Band.Label
		}
		if it.Share != nil && it.Percentile == nil {
			m["share_with_same_value"] = pctStr(*it.Share)
		}
		items = append(items, m)
	}
	out := map[string]any{"compared_with": r.Cohort, "people": r.N, "values": items}
	if len(r.Invalid) > 0 {
		out["ignored_values"] = r.Invalid
	}
	return out
}

func compactSimilar(ds *dataset.Dataset, r *query.SimilarResponse) map[string]any {
	var outcomes []map[string]any
	for _, o := range r.Outcomes {
		outcomes = append(outcomes, map[string]any{
			"outcome": o.Outcome.Label, "among_similar": pctStr(o.Similar.Value),
			"everyone": pctStr(o.Overall.Value), "ratio": o.Ratio,
		})
	}
	var matched []string
	for _, f := range r.Features {
		if c, ok := ds.Column(f); ok {
			matched = append(matched, c.Label)
		}
	}
	// The unmatched numeric columns that differ most from the whole dataset.
	type diff struct {
		text string
		z    float64
	}
	var diffs []diff
	for _, v := range r.Values {
		c, ok := ds.Column(v.Column)
		if !ok || v.Matched || v.Median == nil || c.Stats == nil || c.Stats.Std == 0 || c.Role == dataset.RoleOutcome {
			continue
		}
		z := (*v.Median - c.Stats.Median) / c.Stats.Std
		diffs = append(diffs, diff{
			text: fmt.Sprintf("%s: median %s vs %s overall", c.Label, num(c, *v.Median), num(c, c.Stats.Median)),
			z:    math.Abs(z),
		})
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].z > diffs[j].z })
	var notable []string
	for i := 0; i < len(diffs) && i < 4; i++ {
		if diffs[i].z >= 0.15 {
			notable = append(notable, diffs[i].text)
		}
	}
	return map[string]any{
		"similar_people": r.K, "matched_on": matched, "outcomes": outcomes, "notable_differences": notable,
	}
}

func pValue(p float64) string {
	if p < 0.001 {
		return "< 0.001"
	}
	return fmt.Sprintf("%.3f", p)
}
