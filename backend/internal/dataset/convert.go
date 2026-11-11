package dataset

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	fileFormatName    = "glucolens-columnar"
	fileFormatVersion = 1

	// maxDiscreteLevels is the most distinct integer values a numeric column
	// may have and still be treated as discrete (0–10 scales fit).
	maxDiscreteLevels = 12
	// maxCategoricalLevels is the most distinct labels a text column may have
	// and still be treated as categorical rather than free text.
	maxCategoricalLevels = 60
)

// fileFormat is the on-disk JSON layout: one entry per column.
type fileFormat struct {
	Format  string       `json:"format"`
	Version int          `json:"version"`
	Source  SourceInfo   `json:"source"`
	Rows    int          `json:"rows"`
	Columns []fileColumn `json:"columns"`
}

type fileColumn struct {
	Name     string `json:"name"`
	Header   string `json:"header"`
	Kind     Kind   `json:"kind"`
	Decimals int    `json:"decimals"`
	// Levels holds the labels of categorical/text columns, and the original
	// labels of yes/no text columns (index 0 = no, index 1 = yes).
	Levels []string `json:"levels,omitempty"`
	// Values holds numeric values; rows listed in Missing hold a placeholder 0.
	Values  []float64 `json:"values,omitempty"`
	Missing []int     `json:"missing,omitempty"`
	// Codes holds level indexes for categorical/text columns; -1 is missing.
	Codes []int32 `json:"codes,omitempty"`
}

// utf8BOM is the byte order mark some tools put at the start of a CSV.
var utf8BOM = string([]byte{0xEF, 0xBB, 0xBF})

var missingTokens = map[string]bool{
	"": true, "na": true, "n/a": true, "nan": true, "null": true, "none": true, "?": true, "-": true,
}

var yesNoPairs = []struct{ yes, no string }{
	{"yes", "no"}, {"true", "false"}, {"y", "n"},
}

// rawColumn accumulates one column while the CSV is scanned.
type rawColumn struct {
	header   string
	name     string
	numeric  bool
	nums     []float64
	dict     map[string]int32
	labels   []string
	codes    []int32
	decimals int
	missing  int
}

// ConvertCSV parses the CSV once, infers column types and writes the
// columnar JSON file. It returns the source description stored in the file.
func ConvertCSV(csvPath, jsonPath string) (SourceInfo, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return SourceInfo{}, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	counter := &countingReader{r: io.TeeReader(f, hasher)}
	r := csv.NewReader(counter)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	r.ReuseRecord = true

	header, err := r.Read()
	if err != nil {
		return SourceInfo{}, fmt.Errorf("read csv header: %w", err)
	}
	cols := make([]*rawColumn, len(header))
	used := map[string]int{}
	for i, h := range header {
		h = strings.TrimSpace(strings.TrimPrefix(h, utf8BOM))
		name := snakeCase(h)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		if n := used[name]; n > 0 {
			used[name] = n + 1
			name = fmt.Sprintf("%s_%d", name, n+1)
		} else {
			used[name] = 1
		}
		cols[i] = &rawColumn{header: h, name: name, numeric: true, dict: map[string]int32{}}
	}

	rows := 0
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return SourceInfo{}, fmt.Errorf("read csv row %d: %w", rows+2, err)
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // blank line
		}
		for i, c := range cols {
			var v string
			if i < len(rec) {
				v = strings.TrimSpace(rec[i])
			}
			c.add(v)
		}
		rows++
	}
	if rows == 0 {
		return SourceInfo{}, errors.New("csv has a header but no rows")
	}

	out := fileFormat{
		Format:  fileFormatName,
		Version: fileFormatVersion,
		Rows:    rows,
		Source: SourceInfo{
			File:        filepath.Base(csvPath),
			SHA256:      hex.EncodeToString(hasher.Sum(nil)),
			Bytes:       counter.n,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	for _, c := range cols {
		out.Columns = append(out.Columns, c.finish())
	}

	if err := writeJSONAtomic(jsonPath, out); err != nil {
		return SourceInfo{}, err
	}
	return out.Source, nil
}

func (c *rawColumn) add(v string) {
	if missingTokens[strings.ToLower(v)] {
		c.missing++
		if c.numeric {
			c.nums = append(c.nums, math.NaN())
		}
		c.codes = append(c.codes, -1)
		return
	}
	code, ok := c.dict[v]
	if !ok {
		code = int32(len(c.labels))
		c.dict[v] = code
		c.labels = append(c.labels, v)
	}
	c.codes = append(c.codes, code)

	if c.numeric {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			c.numeric = false
			c.nums = nil
			return
		}
		c.nums = append(c.nums, f)
		if dot := strings.IndexByte(v, '.'); dot >= 0 {
			d := len(v) - dot - 1
			if e := strings.IndexAny(v, "eE"); e > dot {
				d = e - dot - 1
			}
			if d > c.decimals {
				c.decimals = min(d, 4)
			}
		}
	}
}

func (c *rawColumn) finish() fileColumn {
	fc := fileColumn{Name: c.name, Header: c.header, Decimals: c.decimals}
	present := len(c.codes) - c.missing

	switch {
	case present == 0:
		fc.Kind = KindText
		fc.Codes = c.codes
		return fc

	case c.numeric:
		fc.Kind = inferNumericKind(c.nums)
		fc.Values = make([]float64, len(c.nums))
		for i, v := range c.nums {
			if math.IsNaN(v) {
				fc.Missing = append(fc.Missing, i)
				continue
			}
			fc.Values[i] = v
		}
		return fc
	}

	// Text column: a yes/no pair becomes binary, a small label set becomes
	// categorical and anything else stays free text.
	if yes, no, ok := yesNoLabels(c.labels); ok {
		fc.Kind = KindBinary
		fc.Levels = []string{no, yes}
		fc.Values = make([]float64, len(c.codes))
		for i, code := range c.codes {
			switch {
			case code < 0:
				fc.Missing = append(fc.Missing, i)
			case strings.EqualFold(c.labels[code], yes):
				fc.Values[i] = 1
			}
		}
		return fc
	}
	if len(c.labels) <= maxCategoricalLevels {
		fc.Kind = KindCategorical
	} else {
		fc.Kind = KindText
	}
	fc.Levels = c.labels
	fc.Codes = c.codes
	return fc
}

// inferNumericKind classifies a numeric column from its distinct values.
func inferNumericKind(nums []float64) Kind {
	distinct := map[float64]struct{}{}
	allInt := true
	for _, v := range nums {
		if math.IsNaN(v) {
			continue
		}
		if v != math.Trunc(v) {
			allInt = false
		}
		if len(distinct) <= maxDiscreteLevels {
			distinct[v] = struct{}{}
		}
	}
	if len(distinct) <= 2 {
		binary := true
		for v := range distinct {
			if v != 0 && v != 1 {
				binary = false
			}
		}
		if binary {
			return KindBinary
		}
	}
	if allInt && len(distinct) <= maxDiscreteLevels {
		return KindDiscrete
	}
	return KindContinuous
}

// yesNoLabels reports whether the labels are a yes/no style pair and returns
// the original spelling of each side.
func yesNoLabels(labels []string) (yes, no string, ok bool) {
	// Case variants ("Yes", "YES", "yes") count as one label.
	lower := map[string]bool{}
	for _, l := range labels {
		lower[strings.ToLower(l)] = true
	}
	if len(lower) == 0 || len(lower) > 2 {
		return "", "", false
	}
	for _, pair := range yesNoPairs {
		yes, no = "", ""
		match := true
		for _, l := range labels {
			switch strings.ToLower(l) {
			case pair.yes:
				if yes == "" {
					yes = l // keep the first spelling seen
				}
			case pair.no:
				if no == "" {
					no = l
				}
			default:
				match = false
			}
		}
		if match {
			if yes == "" {
				yes = titleCase(pair.yes)
			}
			if no == "" {
				no = titleCase(pair.no)
			}
			return yes, no, true
		}
	}
	return "", "", false
}

// snakeCase turns a CSV header such as "Blood Pressure (mmHg)" into a safe
// column name such as "blood_pressure_mmhg".
func snakeCase(s string) string {
	var b strings.Builder
	lastUnderscore := true
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
		case !lastUnderscore:
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// humanize turns "physical_activity_minutes_per_week" into
// "Physical activity minutes per week".
func humanize(name string) string {
	return titleCase(strings.ReplaceAll(name, "_", " "))
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp json: %w", err)
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("encode json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write json: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("replace json: %w", err)
	}
	return nil
}

// HashFile returns the SHA-256 of a file as hex.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// sortedLevelValues returns the distinct non-missing values of a numeric
// column in ascending order.
func sortedLevelValues(nums []float64) []float64 {
	seen := map[float64]struct{}{}
	for _, v := range nums {
		if !math.IsNaN(v) {
			seen[v] = struct{}{}
		}
	}
	out := make([]float64, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Float64s(out)
	return out
}
