package query

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"glucolens/internal/dataset"
)

const realCSV = "../../data/diabetes_dataset.csv"

var (
	realOnce sync.Once
	realEng  *Engine
	realErr  error
)

func realEngine(t *testing.T) *Engine {
	t.Helper()
	if _, err := os.Stat(realCSV); err != nil {
		t.Skipf("real dataset not available: %v", err)
	}
	realOnce.Do(func() {
		dir, err := os.MkdirTemp("", "glucolens-query-test")
		if err != nil {
			realErr = err
			return
		}
		ds, _, err := dataset.Open(realCSV, filepath.Join(dir, "dataset.json"), false)
		if err != nil {
			realErr = err
			return
		}
		realEng = New(ds)
	})
	if realErr != nil {
		t.Fatal(realErr)
	}
	return realEng
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > tol {
		t.Errorf("%s = %.5f, want %.5f ± %g", what, got, want, tol)
	}
}

func f(v float64) *float64 { return &v }

// mustJSON fails when a response cannot be encoded (for example NaN).
func mustJSON(t *testing.T, v any) {
	t.Helper()
	if _, err := json.Marshal(v); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
}

func TestRiskMatchesIndependentFigures(t *testing.T) {
	e := realEngine(t)
	out := Outcome{Preset: "diagnosed_diabetes"}

	fam, err := e.Risk(RiskRequest{Outcome: out, Factor: "family_history_diabetes"})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, fam)
	near(t, "diabetes rate without family history", fam.Levels[0].Rate.Value, 0.549, 0.0005)
	near(t, "diabetes rate with family history", fam.Levels[1].Rate.Value, 0.783, 0.0005)
	if fam.Levels[0].Rate.N != 78059 || fam.Levels[1].Rate.N != 21941 {
		t.Errorf("family history group sizes = %d, %d", fam.Levels[0].Rate.N, fam.Levels[1].Rate.N)
	}
	if fam.Reference != "No" || !fam.Levels[0].IsReference {
		t.Errorf("reference = %q", fam.Reference)
	}
	rr := fam.Levels[1].RelativeRisk
	if rr == nil || rr.Value < 1.4 || rr.Low > rr.Value || rr.High < rr.Value {
		t.Errorf("relative risk = %+v", rr)
	}
	if fam.Association.Strength != StrengthModerate {
		t.Errorf("family history strength = %s (V=%.3f)", fam.Association.Strength, fam.Association.CramersV)
	}

	smoke, err := e.Risk(RiskRequest{Outcome: out, Factor: "smoking_status"})
	if err != nil {
		t.Fatal(err)
	}
	for _, lv := range smoke.Levels {
		near(t, "diabetes rate for smoking "+lv.Label, lv.Rate.Value, 0.600, 0.0015)
	}
	if smoke.Reference != "Never" || smoke.Association.Strength != StrengthNone {
		t.Errorf("smoking reference=%s strength=%s p=%.3f", smoke.Reference, smoke.Association.Strength, smoke.Association.PValue)
	}
	if !strings.HasPrefix(smoke.Verdict, "No meaningful difference") {
		t.Errorf("smoking verdict = %q", smoke.Verdict)
	}

	age, err := e.Risk(RiskRequest{Outcome: out, Factor: "age"})
	if err != nil {
		t.Fatal(err)
	}
	wantAge := []float64{0.499, 0.566, 0.628, 0.700}
	for i, lv := range age.Levels {
		near(t, "diabetes rate for age "+lv.Label, lv.Rate.Value, wantAge[i], 0.0005)
	}

	bmi, err := e.Risk(RiskRequest{Outcome: out, Factor: "bmi"})
	if err != nil {
		t.Fatal(err)
	}
	if bmi.Grouping != "bands" || bmi.Reference != "Healthy" {
		t.Errorf("bmi grouping=%s reference=%s", bmi.Grouping, bmi.Reference)
	}
	near(t, "diabetes rate overweight", bmi.Levels[2].Rate.Value, 0.620, 0.0005)
	near(t, "diabetes rate obesity", bmi.Levels[3].Rate.Value, 0.683, 0.0005)

	// The HbA1c threshold preset: every diagnosed person in this dataset with
	// HbA1c >= 6.5 is diagnosed, so the rate there is 100%.
	strat, err := e.Risk(RiskRequest{
		Outcome: Outcome{Column: "diagnosed_diabetes"}, Factor: "family_history_diabetes", StratifyBy: "age",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, strat)
	if len(strat.Strata) != 4 || strat.Outcome.ID != "diagnosed_diabetes" {
		t.Errorf("strata = %d, outcome id = %q", len(strat.Strata), strat.Outcome.ID)
	}
	n := 0
	for _, s := range strat.Strata {
		n += s.N
	}
	if n != 100000 {
		t.Errorf("strata cover %d rows, want 100000", n)
	}
}

func TestOutcomeThresholdAndValidation(t *testing.T) {
	e := realEngine(t)
	res, err := e.Risk(RiskRequest{Outcome: Outcome{Column: "hba1c", Min: f(6.5)}, Factor: "diagnosed_diabetes"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Base.Events != 51105 {
		t.Errorf("HbA1c >= 6.5 events = %d, want 51105", res.Base.Events)
	}
	if res.Outcome.ID != "hba1c_diabetes_range" {
		t.Errorf("threshold should resolve to its preset, got %+v", res.Outcome)
	}

	cases := []RiskRequest{
		{Outcome: Outcome{Preset: "nope"}, Factor: "age"},
		{Outcome: Outcome{Column: "diabetes_stage"}, Factor: "age"},
		{Outcome: Outcome{Column: "diabetes_stage", Equals: []string{"Type 9"}}, Factor: "age"},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "not_a_column"},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "diagnosed_diabetes"},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "age", Reference: "Teenagers"},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "age", Filters: []Filter{{Column: "bmi", Equals: []string{"30"}}}},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "age", Filters: []Filter{{Column: "gender", Min: f(1)}}},
		{Outcome: Outcome{Preset: "diagnosed_diabetes"}, Factor: "age", Filters: []Filter{{Column: "age", Min: f(60), Max: f(40)}}},
	}
	for i, c := range cases {
		if _, err := e.Risk(c); err == nil || !IsValidation(err) {
			t.Errorf("case %d: want a validation error, got %v", i, err)
		}
	}
}

func TestDistributionAndCompare(t *testing.T) {
	e := realEngine(t)
	d, err := e.Distribution(DistributionRequest{Column: "hba1c", SplitBy: "family_history_diabetes"})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, d)
	if d.Mode != "histogram" || len(d.Bins) < 20 || len(d.Bins) > 30 {
		t.Errorf("hba1c histogram: mode=%s bins=%d", d.Mode, len(d.Bins))
	}
	total := 0
	for _, c := range d.Overall.Counts {
		total += c
	}
	if total != 100000 || d.N != 100000 {
		t.Errorf("histogram counts %d rows", total)
	}
	if len(d.Groups) != 2 || d.Groups[0].N+d.Groups[1].N != 100000 {
		t.Errorf("split groups = %+v", len(d.Groups))
	}
	if len(d.Bands) != 3 || d.Bands[2].Count != 51105 {
		t.Errorf("hba1c bands = %+v", d.Bands)
	}

	lv, err := e.Distribution(DistributionRequest{Column: "smoking_status", Filters: []Filter{{Column: "gender", Equals: []string{"female"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if lv.Mode != "levels" || lv.N != 50216 || lv.Cohort != "Gender: Female" {
		t.Errorf("smoking among women: mode=%s n=%d cohort=%q", lv.Mode, lv.N, lv.Cohort)
	}

	c, err := e.Compare(CompareRequest{Metric: "hba1c", GroupBy: "family_history_diabetes"})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, c)
	if c.Mode != "numeric" || c.Groups[1].Summary.Median <= c.Groups[0].Summary.Median {
		t.Errorf("hba1c should be higher with a family history: %+v / %+v", c.Groups[0].Summary, c.Groups[1].Summary)
	}
	if c.Test.PValue > 1e-6 || c.Test.Strength == StrengthNone {
		t.Errorf("hba1c by family history test = %+v", c.Test)
	}

	rate, err := e.Compare(CompareRequest{Metric: "diagnosed_diabetes", GroupBy: "smoking_status"})
	if err != nil {
		t.Fatal(err)
	}
	if rate.Mode != "rate" || rate.Test.Strength != StrengthNone {
		t.Errorf("diabetes by smoking: mode=%s test=%+v", rate.Mode, rate.Test)
	}
	shares, err := e.Compare(CompareRequest{Metric: "diabetes_stage", GroupBy: "age"})
	if err != nil {
		t.Fatal(err)
	}
	if shares.Mode != "shares" || len(shares.Groups[0].Shares) != 5 {
		t.Errorf("stage by age: mode=%s", shares.Mode)
	}
}

func TestRankProfileSimilar(t *testing.T) {
	e := realEngine(t)
	rank, err := e.Rank(RankRequest{Outcome: Outcome{Preset: "diagnosed_diabetes"}})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, rank)
	pos := map[string]int{}
	for i, f := range rank.Factors {
		pos[f.Column] = i
		if f.Role == dataset.RoleMarker || f.Role == dataset.RoleOutcome {
			t.Errorf("%s (%s) should not be ranked by default", f.Column, f.Role)
		}
	}
	if pos["family_history_diabetes"] > 2 {
		t.Errorf("family history ranks #%d", pos["family_history_diabetes"]+1)
	}
	if pos["smoking_status"] < len(rank.Factors)/2 {
		t.Errorf("smoking ranks #%d of %d", pos["smoking_status"]+1, len(rank.Factors))
	}
	withMarkers, err := e.Rank(RankRequest{Outcome: Outcome{Preset: "diagnosed_diabetes"}, IncludeMarkers: true})
	if err != nil {
		t.Fatal(err)
	}
	if withMarkers.Factors[0].Column != "hba1c" {
		t.Errorf("with lab markers, HbA1c should rank first; got %s", withMarkers.Factors[0].Column)
	}

	hba1c, _ := e.ds.Column("hba1c")
	prof, err := e.Profile(ProfileRequest{Values: map[string]any{
		"hba1c": hba1c.Stats.Median, "gender": "Female", "family_history_diabetes": 1, "bogus": 3,
	}})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, prof)
	if len(prof.Items) != 3 || prof.Invalid["bogus"] == "" {
		t.Fatalf("profile items=%d invalid=%v", len(prof.Items), prof.Invalid)
	}
	for _, it := range prof.Items {
		switch it.Column {
		case "hba1c":
			near(t, "percentile of the median", *it.Percentile, 50, 1)
			if it.Band == nil || it.Position != "typical" {
				t.Errorf("hba1c item = %+v", it)
			}
		case "gender":
			near(t, "share of women", *it.Share, 0.50216, 0.0001)
		}
	}

	sim, err := e.Similar(SimilarRequest{Values: map[string]any{
		"age": 55, "gender": "Male", "bmi": 31, "family_history_diabetes": "Yes", "physical_activity_minutes_per_week": 60,
	}, K: 500, MatchOn: MatchLifestyle})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, sim)
	if sim.K != 500 || len(sim.Features) != 5 || len(sim.Outcomes) == 0 {
		t.Fatalf("similar: k=%d features=%v outcomes=%d", sim.K, sim.Features, len(sim.Outcomes))
	}
	for _, o := range sim.Outcomes {
		if o.Similar.N != 500 {
			t.Errorf("%s similar n = %d", o.Outcome.ID, o.Similar.N)
		}
		if o.Outcome.ID == "obesity" {
			t.Errorf("BMI was matched on, so the obesity outcome should be skipped")
		}
		if o.Outcome.ID == "diagnosed_diabetes" && o.Similar.Value <= o.Overall.Value {
			t.Errorf("older men with obesity and a family history should have a higher diabetes rate: %+v", o)
		}
	}
	if _, err := e.Similar(SimilarRequest{Values: map[string]any{"age": 40}}); !IsValidation(err) {
		t.Errorf("one value should not be enough: %v", err)
	}
}

func TestCorrelationsAndRelationship(t *testing.T) {
	e := realEngine(t)
	c := e.Correlations()
	mustJSON(t, c)
	idx := map[string]int{}
	for i, col := range c.Columns {
		idx[col.Name] = i
	}
	n := len(c.Columns)
	if n != 24 { // 31 columns minus 7 categorical ones
		t.Errorf("numeric columns = %d, want 24", n)
	}
	for i := 0; i < n; i++ {
		if c.Matrix[i][i] != 1 {
			t.Errorf("diagonal %d = %v", i, c.Matrix[i][i])
		}
		for j := 0; j < n; j++ {
			if c.Matrix[i][j] != c.Matrix[j][i] {
				t.Fatalf("matrix not symmetric at %d,%d", i, j)
			}
		}
	}
	if r := c.Matrix[idx["hba1c"]][idx["glucose_fasting"]]; r < 0.5 {
		t.Errorf("hba1c vs fasting glucose r = %.3f", r)
	}

	rel, err := e.Relationship(RelationshipRequest{X: "bmi", Y: "hba1c"})
	if err != nil {
		t.Fatal(err)
	}
	mustJSON(t, rel)
	sum := 0
	for _, cell := range rel.Cells {
		sum += cell.Count
	}
	if sum != 100000 || rel.N != 100000 || len(rel.Trend) == 0 {
		t.Errorf("relationship cells sum=%d n=%d trend=%d", sum, rel.N, len(rel.Trend))
	}
	if _, err := e.Relationship(RelationshipRequest{X: "gender", Y: "hba1c"}); !IsValidation(err) {
		t.Errorf("categorical x should be rejected: %v", err)
	}
}

// fixtureEngine builds a small dataset with known counts.
func fixtureEngine(t *testing.T, rows []string) *Engine {
	t.Helper()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "f.csv")
	content := "exposure,outcome,score\n" + strings.Join(rows, "\n") + "\n"
	if err := os.WriteFile(csvPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "f.json")
	if _, err := dataset.ConvertCSV(csvPath, jsonPath); err != nil {
		t.Fatal(err)
	}
	ds, err := dataset.LoadWithOverlay(jsonPath, []byte(`{"columns":{"exposure":{"reference":"No"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return New(ds)
}

func TestRiskMathOnFixture(t *testing.T) {
	var rows []string
	add := func(exposure, outcome string, n int) {
		for i := 0; i < n; i++ {
			rows = append(rows, fmt.Sprintf("%s,%s,%d", exposure, outcome, i%7))
		}
	}
	add("Yes", "Yes", 30)
	add("Yes", "No", 70)
	add("No", "Yes", 10)
	add("No", "No", 90)
	e := fixtureEngine(t, rows)

	res, err := e.Risk(RiskRequest{Outcome: Outcome{Column: "outcome"}, Factor: "exposure"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reference != "No" {
		t.Fatalf("reference = %s", res.Reference)
	}
	exposed := res.Levels[1]
	near(t, "exposed rate", exposed.Rate.Value, 0.30, 1e-9)
	// Wilson interval for 30/100.
	near(t, "wilson low", exposed.Rate.Low, 0.2189, 0.0005)
	near(t, "wilson high", exposed.Rate.High, 0.3958, 0.0005)
	// RR = 0.30 / 0.10 = 3, log-method CI.
	near(t, "relative risk", exposed.RelativeRisk.Value, 3, 1e-9)
	// se = sqrt(1/30 - 1/100 + 1/10 - 1/100) = 0.33665
	near(t, "rr low", exposed.RelativeRisk.Low, 1.5508, 0.0005)
	near(t, "rr high", exposed.RelativeRisk.High, 5.8033, 0.0005)
	near(t, "difference", exposed.Difference, 0.20, 1e-9)
	// chi-square for this 2x2 table is 12.5 (phi = 0.25).
	near(t, "chi-square", res.Association.ChiSquare, 12.5, 0.001)
	near(t, "cramers v", res.Association.CramersV, 0.25, 0.0001)
	if res.Association.PValue > 0.001 || res.Association.Strength != StrengthStrong {
		t.Errorf("association = %+v", res.Association)
	}
	if res.Levels[0].LowN || exposed.LowN {
		t.Errorf("groups of 100 should not be flagged low-n")
	}
}

func TestDistributionsOfTestStatistics(t *testing.T) {
	near(t, "chi-square p at 3.841 (df 1)", chiSquarePValue(3.841459, 1), 0.05, 1e-4)
	near(t, "chi-square p at 5.991 (df 2)", chiSquarePValue(5.991465, 2), 0.05, 1e-4)
	near(t, "chi-square p at 0.5 (df 3)", chiSquarePValue(0.5, 3), 0.918891, 1e-4)
	near(t, "F p at 3.3158 (2, 30)", fPValue(3.315830, 2, 30), 0.05, 1e-4)
	// Checked against numerical integration of the F(5, 10) density.
	near(t, "F p at 1 (5, 10)", fPValue(1, 5, 10), 0.46512, 1e-4)
}
