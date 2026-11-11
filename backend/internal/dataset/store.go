package dataset

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

//go:embed columns.meta.json
var defaultOverlay []byte

// Dataset is the loaded, typed, columnar dataset.
type Dataset struct {
	Name        string
	Description string
	Rows        int
	Columns     []*Column
	Sections    []Section
	Outcomes    []OutcomePreset
	Source      SourceInfo

	byName map[string]*Column
}

// Column returns the column with the given name.
func (d *Dataset) Column(name string) (*Column, bool) {
	c, ok := d.byName[strings.TrimSpace(name)]
	return c, ok
}

// ColumnNames lists every column name in dataset order.
func (d *Dataset) ColumnNames() []string {
	names := make([]string, len(d.Columns))
	for i, c := range d.Columns {
		names[i] = c.Name
	}
	return names
}

// OutcomePreset returns the preset with the given id.
func (d *Dataset) OutcomePreset(id string) (OutcomePreset, bool) {
	for _, p := range d.Outcomes {
		if p.ID == id {
			return p, true
		}
	}
	return OutcomePreset{}, false
}

// Row returns one record as a map. Numeric columns (including yes/no
// columns, as 0/1) are numbers; categorical columns are labels; missing
// values are nil.
func (d *Dataset) Row(i int) map[string]any {
	m := make(map[string]any, len(d.Columns))
	for _, c := range d.Columns {
		if c.IsNumeric() {
			if v := c.num[i]; !math.IsNaN(v) {
				m[c.Name] = v
			} else {
				m[c.Name] = nil
			}
			continue
		}
		if code := c.codes[i]; code >= 0 {
			m[c.Name] = c.Levels[code]
		} else {
			m[c.Name] = nil
		}
	}
	return m
}

// OpenResult reports what Open had to do.
type OpenResult struct {
	Converted bool
	Reason    string
}

// Open makes sure the JSON file is current, converting the CSV when the JSON
// is missing, stale or force is set, and then loads the dataset from the
// JSON. The CSV is optional once the JSON exists.
func Open(csvPath, jsonPath string, force bool) (*Dataset, OpenResult, error) {
	_, jsonErr := os.Stat(jsonPath)
	_, csvErr := os.Stat(csvPath)

	reason := ""
	switch {
	case force:
		reason = "conversion forced"
	case jsonErr != nil:
		reason = "no converted dataset found"
	}
	if reason != "" {
		if csvErr != nil {
			return nil, OpenResult{}, fmt.Errorf("%s and the CSV is not readable: %w", reason, csvErr)
		}
		if _, err := ConvertCSV(csvPath, jsonPath); err != nil {
			return nil, OpenResult{}, err
		}
		ds, err := Load(jsonPath)
		return ds, OpenResult{Converted: true, Reason: reason}, err
	}

	ds, err := Load(jsonPath)
	if err != nil {
		if csvErr != nil {
			return nil, OpenResult{}, err
		}
		// A corrupt or outdated file is regenerated from the CSV.
		if _, cerr := ConvertCSV(csvPath, jsonPath); cerr != nil {
			return nil, OpenResult{}, cerr
		}
		ds, err = Load(jsonPath)
		return ds, OpenResult{Converted: true, Reason: "converted dataset was unreadable"}, err
	}
	if csvErr == nil {
		hash, err := HashFile(csvPath)
		if err == nil && hash != ds.Source.SHA256 {
			if _, err := ConvertCSV(csvPath, jsonPath); err != nil {
				return nil, OpenResult{}, err
			}
			ds, err = Load(jsonPath)
			return ds, OpenResult{Converted: true, Reason: "the CSV changed"}, err
		}
	}
	return ds, OpenResult{}, nil
}

// Load reads the columnar JSON file and applies the built-in column overlay.
func Load(jsonPath string) (*Dataset, error) {
	return LoadWithOverlay(jsonPath, defaultOverlay)
}

// LoadWithOverlay reads the columnar JSON file and applies the given overlay.
func LoadWithOverlay(jsonPath string, overlay []byte) (*Dataset, error) {
	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("open dataset json: %w", err)
	}
	defer f.Close()
	return LoadReader(f, overlay)
}

// LoadBytes reads an already converted JSON dataset from memory (used by the
// WebAssembly build, where the browser fetches dataset.json).
func LoadBytes(data []byte) (*Dataset, error) {
	return LoadReader(bytes.NewReader(data), defaultOverlay)
}

// LoadReader reads a converted JSON dataset and applies the overlay.
func LoadReader(r io.Reader, overlay []byte) (*Dataset, error) {
	var ff fileFormat
	if err := json.NewDecoder(r).Decode(&ff); err != nil {
		return nil, fmt.Errorf("decode dataset json: %w", err)
	}
	if ff.Format != fileFormatName || ff.Version != fileFormatVersion {
		return nil, fmt.Errorf("dataset json has format %q v%d, want %q v%d", ff.Format, ff.Version, fileFormatName, fileFormatVersion)
	}
	var ov overlayFile
	if len(overlay) > 0 {
		if err := json.Unmarshal(overlay, &ov); err != nil {
			return nil, fmt.Errorf("decode column overlay: %w", err)
		}
	}
	return build(ff, ov)
}

type overlayFile struct {
	Dataset struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"dataset"`
	Sections []Section                `json:"sections"`
	Columns  map[string]overlayColumn `json:"columns"`
	Outcomes []OutcomePreset          `json:"outcomes"`
}

type overlayColumn struct {
	Label       string   `json:"label"`
	Short       string   `json:"short"`
	Unit        string   `json:"unit"`
	Section     string   `json:"section"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Decimals    *int     `json:"decimals"`
	Levels      []string `json:"levels"`
	Order       []string `json:"order"`
	Ordered     bool     `json:"ordered"`
	Reference   string   `json:"reference"`
	Bands       []Band   `json:"bands"`
	Groups      []Range  `json:"groups"`
}

func build(ff fileFormat, ov overlayFile) (*Dataset, error) {
	if ff.Rows <= 0 || len(ff.Columns) == 0 {
		return nil, errors.New("dataset json is empty")
	}
	ds := &Dataset{
		Name:        ov.Dataset.Name,
		Description: ov.Dataset.Description,
		Rows:        ff.Rows,
		Source:      ff.Source,
		byName:      map[string]*Column{},
	}
	if ds.Name == "" {
		ds.Name = strings.TrimSuffix(ff.Source.File, ".csv")
	}

	for _, fc := range ff.Columns {
		col, err := buildColumn(ff.Rows, fc, ov.Columns[fc.Name])
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", fc.Name, err)
		}
		ds.Columns = append(ds.Columns, col)
		ds.byName[col.Name] = col
	}

	ds.Sections = buildSections(ds.Columns, ov.Sections)
	ds.Outcomes = buildOutcomes(ds, ov.Outcomes)
	return ds, nil
}

func buildColumn(rows int, fc fileColumn, oc overlayColumn) (*Column, error) {
	c := &Column{
		Name:        fc.Name,
		Kind:        fc.Kind,
		Decimals:    fc.Decimals,
		Label:       firstNonEmpty(oc.Label, prettyHeader(fc.Header, fc.Name)),
		Unit:        oc.Unit,
		Section:     firstNonEmpty(oc.Section, "other"),
		Role:        firstNonEmpty(oc.Role, RoleOther),
		Description: oc.Description,
		Ordered:     oc.Ordered,
		Bands:       oc.Bands,
		Groups:      oc.Groups,
	}
	c.Short = firstNonEmpty(oc.Short, c.Label)
	if oc.Decimals != nil {
		c.Decimals = *oc.Decimals
	}

	switch c.Kind {
	case KindBinary, KindDiscrete, KindContinuous:
		if len(fc.Values) != rows {
			return nil, fmt.Errorf("has %d values, want %d", len(fc.Values), rows)
		}
		c.num = fc.Values
		for _, i := range fc.Missing {
			if i >= 0 && i < rows {
				c.num[i] = math.NaN()
			}
		}
		c.Missing = len(fc.Missing)
	case KindCategorical, KindText:
		if len(fc.Codes) != rows {
			return nil, fmt.Errorf("has %d codes, want %d", len(fc.Codes), rows)
		}
		c.codes = fc.Codes
		c.Levels = append([]string(nil), fc.Levels...)
		for _, code := range c.codes {
			if code < 0 {
				c.Missing++
			}
		}
	default:
		return nil, fmt.Errorf("unknown kind %q", c.Kind)
	}

	switch c.Kind {
	case KindBinary:
		c.Levels = []string{"No", "Yes"}
		if len(fc.Levels) == 2 {
			c.Levels = fc.Levels
		}
		if len(oc.Levels) == 2 {
			c.Levels = oc.Levels
		}
		c.levelValues = []float64{0, 1}
	case KindDiscrete:
		c.levelValues = sortedLevelValues(c.num)
		c.Levels = make([]string, len(c.levelValues))
		for i, v := range c.levelValues {
			c.Levels[i] = strconv.FormatFloat(v, 'f', -1, 64)
		}
	case KindCategorical:
		c.reorderLevels(oc.Order)
	}
	if c.Kind == KindBinary || c.Kind == KindDiscrete {
		c.levelIndex = make(map[float64]int, len(c.levelValues))
		for i, v := range c.levelValues {
			c.levelIndex[v] = i
		}
	}

	if c.HasLevels() {
		c.Counts = make([]int, len(c.Levels))
		for i := 0; i < rows; i++ {
			if l := c.Level(i); l >= 0 {
				c.Counts[l]++
			}
		}
		if oc.Reference != "" {
			if idx, ok := c.FindLevel(oc.Reference); ok {
				c.Reference = c.Levels[idx]
			}
		}
	}
	if c.IsNumeric() {
		c.computeStats()
	}
	if c.Kind == KindContinuous && c.Reference == "" && oc.Reference != "" {
		// For continuous columns the reference names one of the groups/bands.
		c.Reference = oc.Reference
	}
	return c, nil
}

// reorderLevels puts categorical levels in the overlay's order, followed by
// any unlisted levels from most to least common, and remaps the codes.
func (c *Column) reorderLevels(order []string) {
	counts := make([]int, len(c.Levels))
	for _, code := range c.codes {
		if code >= 0 {
			counts[code]++
		}
	}
	var newOrder []int
	placed := make([]bool, len(c.Levels))
	for _, want := range order {
		for i, l := range c.Levels {
			if !placed[i] && strings.EqualFold(l, want) {
				newOrder = append(newOrder, i)
				placed[i] = true
			}
		}
	}
	var rest []int
	for i := range c.Levels {
		if !placed[i] {
			rest = append(rest, i)
		}
	}
	sort.SliceStable(rest, func(a, b int) bool { return counts[rest[a]] > counts[rest[b]] })
	newOrder = append(newOrder, rest...)

	remap := make([]int32, len(c.Levels))
	levels := make([]string, len(c.Levels))
	for newIdx, oldIdx := range newOrder {
		remap[oldIdx] = int32(newIdx)
		levels[newIdx] = c.Levels[oldIdx]
	}
	codes := make([]int32, len(c.codes))
	for i, code := range c.codes {
		if code < 0 {
			codes[i] = -1
		} else {
			codes[i] = remap[code]
		}
	}
	c.codes = codes
	c.Levels = levels
}

func (c *Column) computeStats() {
	vals := make([]float64, 0, len(c.num))
	sum := 0.0
	for _, v := range c.num {
		if !math.IsNaN(v) {
			vals = append(vals, v)
			sum += v
		}
	}
	sort.Float64s(vals)
	c.sorted = vals
	n := len(vals)
	if n == 0 {
		return
	}
	mean := sum / float64(n)
	ss := 0.0
	for _, v := range vals {
		ss += (v - mean) * (v - mean)
	}
	std := 0.0
	if n > 1 {
		std = math.Sqrt(ss / float64(n-1))
	}
	c.Stats = &Stats{
		Count:   n,
		Missing: c.Missing,
		Min:     vals[0],
		Max:     vals[n-1],
		Mean:    mean,
		Std:     std,
		P5:      Quantile(vals, 0.05),
		P25:     Quantile(vals, 0.25),
		Median:  Quantile(vals, 0.5),
		P75:     Quantile(vals, 0.75),
		P95:     Quantile(vals, 0.95),
	}
	step := math.Pow(10, -float64(c.Decimals))
	c.Input = &InputSpec{Min: vals[0], Max: vals[n-1], Step: step}
}

// Quantile returns the p-quantile of sorted values using linear
// interpolation between closest ranks.
func Quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	if n == 1 || p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	h := float64(n-1) * p
	lo := int(math.Floor(h))
	if lo+1 >= n {
		return sorted[n-1]
	}
	return sorted[lo] + (h-float64(lo))*(sorted[lo+1]-sorted[lo])
}

func buildSections(cols []*Column, defs []Section) []Section {
	byID := map[string]*Section{}
	var order []string
	for _, d := range defs {
		s := Section{ID: d.ID, Label: d.Label, Description: d.Description}
		byID[d.ID] = &s
		order = append(order, d.ID)
	}
	for _, c := range cols {
		s, ok := byID[c.Section]
		if !ok {
			s = &Section{ID: c.Section, Label: humanize(c.Section)}
			byID[c.Section] = s
			order = append(order, c.Section)
		}
		s.Columns = append(s.Columns, c.Name)
	}
	var out []Section
	for _, id := range order {
		if s := byID[id]; len(s.Columns) > 0 {
			out = append(out, *s)
		}
	}
	return out
}

func buildOutcomes(ds *Dataset, presets []OutcomePreset) []OutcomePreset {
	var out []OutcomePreset
	for _, p := range presets {
		if err := ds.ValidateOutcome(p.Outcome); err == nil {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out
	}
	// No usable presets (a different CSV): offer every yes/no column.
	for _, c := range ds.Columns {
		if c.Kind == KindBinary {
			out = append(out, OutcomePreset{
				ID:      c.Name,
				Label:   c.Label,
				Outcome: OutcomeDef{Column: c.Name, Equals: []string{c.Levels[1]}},
			})
		}
	}
	return out
}

// ValidateOutcome checks that an outcome definition refers to a real column
// and real values.
func (d *Dataset) ValidateOutcome(o OutcomeDef) error {
	c, ok := d.Column(o.Column)
	if !ok {
		return fmt.Errorf("unknown outcome column %q", o.Column)
	}
	if !c.Analysable() {
		return fmt.Errorf("column %q cannot be analysed", o.Column)
	}
	if len(o.Equals) > 0 {
		if !c.HasLevels() {
			return fmt.Errorf("outcome column %q is numeric; use min/max instead of values", o.Column)
		}
		for _, v := range o.Equals {
			if _, ok := c.FindLevel(v); !ok {
				return fmt.Errorf("%q is not a value of %s (valid: %s)", v, o.Column, strings.Join(c.Levels, ", "))
			}
		}
		return nil
	}
	if o.Min != nil || o.Max != nil {
		if !c.IsNumeric() {
			return fmt.Errorf("outcome column %q is categorical; use values instead of min/max", o.Column)
		}
		return nil
	}
	if c.Kind == KindBinary {
		return nil // defaults to the "yes" level
	}
	return fmt.Errorf("outcome on %q needs values (one of %s) or a min/max threshold", o.Column, strings.Join(c.Levels, ", "))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func prettyHeader(header, name string) string {
	h := strings.TrimSpace(header)
	if h == "" || h == name || strings.ContainsRune(h, '_') {
		return humanize(name)
	}
	return h
}
