package query

import (
	"math"
	"sort"
	"strings"

	"glucolens/internal/dataset"
)

// ProfileRequest places a person's values within a cohort.
type ProfileRequest struct {
	Values  map[string]any `json:"values"`
	Filters []Filter       `json:"filters,omitempty"`
	Columns []string       `json:"columns,omitempty"`
}

// ProfileItem describes one of the person's values against the cohort.
type ProfileItem struct {
	Column     string        `json:"column"`
	Label      string        `json:"label"`
	Section    string        `json:"section"`
	Kind       dataset.Kind  `json:"kind"`
	Unit       string        `json:"unit,omitempty"`
	Value      any           `json:"value"`
	Display    string        `json:"display"`
	Percentile *float64      `json:"percentile,omitempty"`
	Median     *float64      `json:"median,omitempty"`
	P25        *float64      `json:"p25,omitempty"`
	P75        *float64      `json:"p75,omitempty"`
	Share      *float64      `json:"share,omitempty"`
	Position   string        `json:"position,omitempty"` // lower | typical | higher
	Band       *dataset.Band `json:"band,omitempty"`
	Tone       string        `json:"tone,omitempty"`
}

// ProfileResponse lists the person's values in dataset order.
type ProfileResponse struct {
	Cohort  string            `json:"cohort"`
	N       int               `json:"n"`
	Items   []ProfileItem     `json:"items"`
	Invalid map[string]string `json:"invalid,omitempty"`
}

// Profile computes percentiles, shares and clinical bands for each value.
func (e *Engine) Profile(req ProfileRequest) (*ProfileResponse, error) {
	vals, invalid := e.ds.NormalizeAll(req.Values)
	if len(vals) == 0 {
		if len(invalid) > 0 {
			return nil, invalidf("none of the values could be used: %s", describeInvalid(invalid))
		}
		return nil, invalidf("no values given")
	}
	rows, err := e.Select(req.Filters)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, c := range req.Columns {
		if _, ok := e.ds.Column(c); !ok {
			return nil, invalidf("unknown column %q", c)
		}
		want[c] = true
	}

	resp := &ProfileResponse{Cohort: e.DescribeFilters(req.Filters), N: len(rows), Invalid: invalid}
	everyone := len(req.Filters) == 0 || len(rows) == e.ds.Rows
	for _, c := range e.ds.Columns {
		v, ok := vals[c.Name]
		if !ok || (len(want) > 0 && !want[c.Name]) {
			continue
		}
		item := ProfileItem{
			Column: c.Name, Label: c.Label, Section: c.Section, Kind: c.Kind, Unit: c.Unit,
			Value: v.JSON(), Display: v.Display,
		}
		if c.IsNumeric() && c.Kind != dataset.KindBinary {
			var sorted []float64
			if everyone {
				sorted = c.Sorted()
			} else {
				sorted = make([]float64, 0, len(rows))
				for _, r := range rows {
					if x := c.Num(int(r)); !math.IsNaN(x) {
						sorted = append(sorted, x)
					}
				}
				sort.Float64s(sorted)
			}
			if len(sorted) > 0 {
				below := sort.SearchFloat64s(sorted, v.Num)
				upto := sort.Search(len(sorted), func(i int) bool { return sorted[i] > v.Num })
				p := (float64(below) + 0.5*float64(upto-below)) / float64(len(sorted)) * 100
				item.Percentile = ptr(round(p, 1))
				q25, med, q75 := dataset.Quantile(sorted, 0.25), dataset.Quantile(sorted, 0.5), dataset.Quantile(sorted, 0.75)
				item.Median, item.P25, item.P75 = ptr(med), ptr(q25), ptr(q75)
				switch {
				case v.Num < q25:
					item.Position = "lower"
				case v.Num > q75:
					item.Position = "higher"
				default:
					item.Position = "typical"
				}
			}
			if b := c.BandOf(v.Num, vals); b != nil {
				item.Band, item.Tone = b, b.Tone
			}
		}
		if c.HasLevels() && v.Level >= 0 && len(rows) > 0 {
			same := 0
			for _, r := range rows {
				if c.Level(int(r)) == v.Level {
					same++
				}
			}
			item.Share = ptr(round(float64(same)/float64(len(rows)), 4))
		}
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}

func describeInvalid(invalid map[string]string) string {
	keys := make([]string, 0, len(invalid))
	for k := range invalid {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + invalid[k]
	}
	return strings.Join(parts, "; ")
}
