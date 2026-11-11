package dataset

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const realCSV = "../../data/diabetes_dataset.csv"

var (
	realOnce sync.Once
	realDS   *Dataset
	realErr  error
)

func loadReal(t *testing.T) *Dataset {
	t.Helper()
	if _, err := os.Stat(realCSV); err != nil {
		t.Skipf("real dataset not available: %v", err)
	}
	realOnce.Do(func() {
		dir, err := os.MkdirTemp("", "glucolens-dataset-test")
		if err != nil {
			realErr = err
			return
		}
		jsonPath := filepath.Join(dir, "dataset.json")
		if _, err := ConvertCSV(realCSV, jsonPath); err != nil {
			realErr = err
			return
		}
		realDS, realErr = Load(jsonPath)
	})
	if realErr != nil {
		t.Fatalf("load real dataset: %v", realErr)
	}
	return realDS
}

func TestRealDatasetShape(t *testing.T) {
	ds := loadReal(t)
	if ds.Rows != 100000 {
		t.Fatalf("rows = %d, want 100000", ds.Rows)
	}
	if len(ds.Columns) != 31 {
		t.Fatalf("columns = %d, want 31", len(ds.Columns))
	}

	wantKinds := map[string]Kind{
		"age": KindContinuous, "gender": KindCategorical, "ethnicity": KindCategorical,
		"education_level": KindCategorical, "income_level": KindCategorical,
		"employment_status": KindCategorical, "smoking_status": KindCategorical,
		"alcohol_consumption_per_week": KindDiscrete, "physical_activity_minutes_per_week": KindContinuous,
		"diet_score": KindContinuous, "sleep_hours_per_day": KindContinuous,
		"screen_time_hours_per_day": KindContinuous, "family_history_diabetes": KindBinary,
		"hypertension_history": KindBinary, "cardiovascular_history": KindBinary,
		"bmi": KindContinuous, "waist_to_hip_ratio": KindContinuous, "systolic_bp": KindContinuous,
		"diastolic_bp": KindContinuous, "heart_rate": KindContinuous, "cholesterol_total": KindContinuous,
		"hdl_cholesterol": KindContinuous, "ldl_cholesterol": KindContinuous, "triglycerides": KindContinuous,
		"glucose_fasting": KindContinuous, "glucose_postprandial": KindContinuous,
		"insulin_level": KindContinuous, "hba1c": KindContinuous, "diabetes_risk_score": KindContinuous,
		"diabetes_stage": KindCategorical, "diagnosed_diabetes": KindBinary,
	}
	for name, want := range wantKinds {
		c, ok := ds.Column(name)
		if !ok {
			t.Errorf("missing column %s", name)
			continue
		}
		if c.Kind != want {
			t.Errorf("%s kind = %s, want %s", name, c.Kind, want)
		}
		if c.Missing != 0 {
			t.Errorf("%s missing = %d, want 0", name, c.Missing)
		}
		if c.Section == "other" {
			t.Errorf("%s has no section from the overlay", name)
		}
	}

	age, _ := ds.Column("age")
	if math.Abs(age.Stats.Mean-50.12) > 0.005 {
		t.Errorf("mean age = %.4f, want 50.12", age.Stats.Mean)
	}
	if age.Stats.Min != 18 || age.Stats.Max != 90 {
		t.Errorf("age range = %v–%v, want 18–90", age.Stats.Min, age.Stats.Max)
	}

	assertCounts(t, ds, "smoking_status", map[string]int{"Never": 59813, "Current": 20176, "Former": 20011})
	assertCounts(t, ds, "diabetes_stage", map[string]int{
		"No Diabetes": 7981, "Pre-Diabetes": 31845, "Type 1": 122, "Type 2": 59774, "Gestational": 278,
	})
	assertCounts(t, ds, "gender", map[string]int{"Female": 50216, "Male": 47771, "Other": 2013})

	smoking, _ := ds.Column("smoking_status")
	if got := smoking.Levels; got[0] != "Never" || got[1] != "Former" || got[2] != "Current" {
		t.Errorf("smoking levels = %v, want overlay order Never, Former, Current", got)
	}
	fam, _ := ds.Column("family_history_diabetes")
	if fam.Levels[0] != "No" || fam.Levels[1] != "Yes" || fam.Counts[1] != 21941 {
		t.Errorf("family history levels=%v counts=%v", fam.Levels, fam.Counts)
	}
	alcohol, _ := ds.Column("alcohol_consumption_per_week")
	if len(alcohol.Levels) != 11 || alcohol.Levels[0] != "0" || alcohol.Levels[10] != "10" {
		t.Errorf("alcohol levels = %v", alcohol.Levels)
	}
	hba1c, _ := ds.Column("hba1c")
	if hba1c.Decimals != 2 || hba1c.Unit != "%" || len(hba1c.Bands) != 3 {
		t.Errorf("hba1c decimals=%d unit=%q bands=%d", hba1c.Decimals, hba1c.Unit, len(hba1c.Bands))
	}

	if len(ds.Sections) != 8 {
		t.Errorf("sections = %d, want 8", len(ds.Sections))
	}
	if len(ds.Outcomes) != 9 {
		t.Errorf("outcome presets = %d, want 9", len(ds.Outcomes))
	}

	row := ds.Row(0)
	if row["age"] != 58.0 || row["gender"] != "Male" || row["hba1c"] != 8.18 || row["diabetes_stage"] != "Type 2" {
		t.Errorf("row 0 = %v", row)
	}
}

func assertCounts(t *testing.T, ds *Dataset, name string, want map[string]int) {
	t.Helper()
	c, _ := ds.Column(name)
	for i, l := range c.Levels {
		if c.Counts[i] != want[l] {
			t.Errorf("%s[%s] = %d, want %d", name, l, c.Counts[i], want[l])
		}
	}
}

func TestInferenceOnFixture(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "fixture.csv")
	content := utf8BOM + "Patient ID,Smoker,Score,Weight (kg),Colour,Empty\n" +
		"p1,Yes,0,70.5,red,\n" +
		"p2,no,3,NA,blue,\n" +
		"p3,YES,5,80,green,\n" +
		"p4,No,2,65.25,red,\n"
	if err := os.WriteFile(csvPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "fixture.json")
	if _, err := ConvertCSV(csvPath, jsonPath); err != nil {
		t.Fatal(err)
	}
	ds, err := LoadWithOverlay(jsonPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]Kind{
		"patient_id": KindCategorical, "smoker": KindBinary, "score": KindDiscrete,
		"weight_kg": KindContinuous, "colour": KindCategorical, "empty": KindText,
	}
	for name, k := range want {
		c, ok := ds.Column(name)
		if !ok {
			t.Fatalf("missing column %s (have %v)", name, ds.ColumnNames())
		}
		if c.Kind != k {
			t.Errorf("%s kind = %s, want %s", name, c.Kind, k)
		}
	}
	smoker, _ := ds.Column("smoker")
	if smoker.Counts[1] != 2 || smoker.Counts[0] != 2 {
		t.Errorf("smoker counts = %v", smoker.Counts)
	}
	weight, _ := ds.Column("weight_kg")
	if weight.Missing != 1 || weight.Decimals != 2 || weight.Stats.Count != 3 {
		t.Errorf("weight missing=%d decimals=%d count=%d", weight.Missing, weight.Decimals, weight.Stats.Count)
	}
	if !math.IsNaN(weight.Num(1)) {
		t.Errorf("weight row 1 should be missing")
	}
	if weight.Label != "Weight (kg)" {
		t.Errorf("weight label = %q", weight.Label)
	}
	// Without presets, every yes/no column becomes an outcome.
	if len(ds.Outcomes) != 1 || ds.Outcomes[0].Outcome.Column != "smoker" {
		t.Errorf("fallback outcomes = %+v", ds.Outcomes)
	}
}

func TestOpenConvertsOnceAndOnChange(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "d.csv")
	jsonPath := filepath.Join(dir, "d.json")
	write := func(s string) {
		if err := os.WriteFile(csvPath, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a,b\n1,x\n2,y\n")

	ds, res, err := Open(csvPath, jsonPath, false)
	if err != nil || !res.Converted || ds.Rows != 2 {
		t.Fatalf("first open: rows=%v res=%+v err=%v", ds, res, err)
	}
	_, res, err = Open(csvPath, jsonPath, false)
	if err != nil || res.Converted {
		t.Fatalf("second open should reuse json: res=%+v err=%v", res, err)
	}
	write("a,b\n1,x\n2,y\n3,z\n")
	ds, res, err = Open(csvPath, jsonPath, false)
	if err != nil || !res.Converted || res.Reason != "the CSV changed" || ds.Rows != 3 {
		t.Fatalf("changed csv: rows=%d res=%+v err=%v", ds.Rows, res, err)
	}
	// The CSV is not needed once the JSON exists.
	if err := os.Remove(csvPath); err != nil {
		t.Fatal(err)
	}
	ds, res, err = Open(csvPath, jsonPath, false)
	if err != nil || res.Converted || ds.Rows != 3 {
		t.Fatalf("json only: res=%+v err=%v", res, err)
	}
}

func TestNormalizeAndBands(t *testing.T) {
	ds := loadReal(t)
	vals, errs := ds.NormalizeAll(map[string]any{
		"gender":                       "female",
		"family_history_diabetes":      "Yes",
		"hypertension_history":         0.0,
		"hba1c":                        "6.1",
		"alcohol_consumption_per_week": 3,
		"hdl_cholesterol":              45,
		"ethnicity":                    "Martian",
		"not_a_column":                 1,
		"bmi":                          "",
	})
	if v := vals["gender"]; v.Display != "Female" {
		t.Errorf("gender = %+v", v)
	}
	if v := vals["family_history_diabetes"]; v.Num != 1 || v.Display != "Yes" {
		t.Errorf("family history = %+v", v)
	}
	if v := vals["hypertension_history"]; v.Num != 0 || v.Display != "No" {
		t.Errorf("hypertension = %+v", v)
	}
	if v := vals["hba1c"]; v.Num != 6.1 || v.Display != "6.1%" {
		t.Errorf("hba1c = %+v", v)
	}
	if v := vals["alcohol_consumption_per_week"]; v.Level != 3 {
		t.Errorf("alcohol level = %d", v.Level)
	}
	if _, ok := errs["ethnicity"]; !ok {
		t.Errorf("expected an error for an unknown ethnicity")
	}
	if errs["not_a_column"] != "unknown column" {
		t.Errorf("errs = %v", errs)
	}
	if _, ok := vals["bmi"]; ok {
		t.Errorf("blank values should be skipped")
	}

	hdl, _ := ds.Column("hdl_cholesterol")
	if b := hdl.BandOf(45, vals); b == nil || b.Label != "Low" {
		t.Errorf("HDL 45 for a woman should be Low, got %+v", b)
	}
	if b := hdl.BandOf(45, nil); b == nil || b.Label != "40–59" {
		t.Errorf("HDL 45 with no gender should use general bands, got %+v", b)
	}
	hba1c, _ := ds.Column("hba1c")
	if b := hba1c.BandOf(6.5, nil); b == nil || b.Label != "Diabetes range" {
		t.Errorf("HbA1c 6.5 band = %+v", b)
	}
}
