// Package dataset turns the checkup CSV into a typed, columnar, in-memory
// dataset. The CSV is converted once into a JSON file; the running app only
// ever reads that JSON.
package dataset

import (
	"math"
	"strconv"
	"strings"
)

// Kind is the analytical type of a column, inferred from its values.
type Kind string

const (
	// KindBinary holds two values stored as 0/1 (for example a yes/no flag).
	KindBinary Kind = "binary"
	// KindDiscrete is numeric with a handful of integer values (for example 0–10).
	KindDiscrete Kind = "discrete"
	// KindContinuous is any other numeric column.
	KindContinuous Kind = "continuous"
	// KindCategorical holds text labels with a limited number of values.
	KindCategorical Kind = "categorical"
	// KindText holds free text or identifiers; it is kept but not analysed.
	KindText Kind = "text"
)

// Roles group columns by meaning. They decide which columns count as risk
// factors, which are lab markers, and which are outcomes.
const (
	RoleDemographic = "demographic"
	RoleLifestyle   = "lifestyle"
	RoleHistory     = "history"
	RoleBody        = "body"
	RoleVitals      = "vitals"
	RoleLipids      = "lipids"
	RoleMarker      = "marker"
	RoleOutcome     = "outcome"
	RoleOther       = "other"
)

// Condition restricts a band to people with a given value in another column,
// for example HDL thresholds that differ by gender.
type Condition struct {
	Column string `json:"column"`
	Equals string `json:"equals"`
}

// Band is a labelled value range. Min is inclusive and Max is exclusive.
type Band struct {
	Label string     `json:"label"`
	Min   *float64   `json:"min,omitempty"`
	Max   *float64   `json:"max,omitempty"`
	Tone  string     `json:"tone"` // ok | watch | alert | info
	When  *Condition `json:"when,omitempty"`
}

// Contains reports whether v falls inside the band.
func (b Band) Contains(v float64) bool {
	if b.Min != nil && v < *b.Min {
		return false
	}
	if b.Max != nil && v >= *b.Max {
		return false
	}
	return true
}

// Range is a labelled bucket used to group a continuous column (age bands).
// Min is inclusive and Max is exclusive.
type Range struct {
	Label string   `json:"label"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// Contains reports whether v falls inside the range.
func (r Range) Contains(v float64) bool {
	if r.Min != nil && v < *r.Min {
		return false
	}
	if r.Max != nil && v >= *r.Max {
		return false
	}
	return true
}

// Stats summarises a numeric column over the whole dataset.
type Stats struct {
	Count   int     `json:"count"`
	Missing int     `json:"missing"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Mean    float64 `json:"mean"`
	Std     float64 `json:"std"`
	P5      float64 `json:"p5"`
	P25     float64 `json:"p25"`
	Median  float64 `json:"median"`
	P75     float64 `json:"p75"`
	P95     float64 `json:"p95"`
}

// InputSpec tells the UI how to render a number input for the column.
type InputSpec struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`
}

// Column is one dataset column: metadata plus its values.
type Column struct {
	Name        string     `json:"name"`
	Label       string     `json:"label"`
	Short       string     `json:"short"`
	Kind        Kind       `json:"kind"`
	Section     string     `json:"section"`
	Role        string     `json:"role"`
	Unit        string     `json:"unit,omitempty"`
	Description string     `json:"description,omitempty"`
	Decimals    int        `json:"decimals"`
	Levels      []string   `json:"levels,omitempty"`
	Counts      []int      `json:"counts,omitempty"`
	Reference   string     `json:"reference,omitempty"`
	Ordered     bool       `json:"ordered,omitempty"`
	Bands       []Band     `json:"bands,omitempty"`
	Groups      []Range    `json:"groups,omitempty"`
	Stats       *Stats     `json:"stats,omitempty"`
	Input       *InputSpec `json:"input,omitempty"`
	Missing     int        `json:"missing"`

	num         []float64       // binary, discrete, continuous; NaN = missing
	codes       []int32         // categorical, text; -1 = missing
	sorted      []float64       // sorted non-missing numeric values
	levelIndex  map[float64]int // discrete and binary: value -> level index
	levelValues []float64       // discrete and binary: level index -> value
}

// IsNumeric reports whether the column stores numbers.
func (c *Column) IsNumeric() bool {
	return c.Kind == KindBinary || c.Kind == KindDiscrete || c.Kind == KindContinuous
}

// HasLevels reports whether the column has a small set of named values that
// can be counted and used to form groups.
func (c *Column) HasLevels() bool {
	return c.Kind == KindBinary || c.Kind == KindDiscrete || c.Kind == KindCategorical
}

// Analysable reports whether the column takes part in analysis at all.
func (c *Column) Analysable() bool { return c.Kind != KindText }

// Num returns the numeric value of row i (NaN when missing or non-numeric).
func (c *Column) Num(i int) float64 {
	if c.num == nil {
		return math.NaN()
	}
	return c.num[i]
}

// Level returns the level index of row i, or -1 when missing or when the
// column has no levels.
func (c *Column) Level(i int) int {
	switch c.Kind {
	case KindCategorical, KindText:
		return int(c.codes[i])
	case KindBinary, KindDiscrete:
		v := c.num[i]
		if math.IsNaN(v) {
			return -1
		}
		if idx, ok := c.levelIndex[v]; ok {
			return idx
		}
	}
	return -1
}

// Sorted returns the sorted non-missing values of a numeric column. Callers
// must not modify the slice.
func (c *Column) Sorted() []float64 { return c.sorted }

// LevelValue returns the numeric value behind level idx of a binary or
// discrete column.
func (c *Column) LevelValue(idx int) float64 {
	if idx < 0 || idx >= len(c.levelValues) {
		return math.NaN()
	}
	return c.levelValues[idx]
}

// FindLevel matches user input against the column's levels. It accepts the
// level label in any case, and for yes/no columns also 0/1 and true/false.
func (c *Column) FindLevel(s string) (int, bool) {
	s = strings.TrimSpace(s)
	for i, l := range c.Levels {
		if strings.EqualFold(l, s) {
			return i, true
		}
	}
	switch c.Kind {
	case KindBinary:
		switch strings.ToLower(s) {
		case "1", "1.0", "true", "yes", "y":
			return c.levelIndex[1], true
		case "0", "0.0", "false", "no", "n":
			return c.levelIndex[0], true
		}
	case KindDiscrete:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if idx, ok := c.levelIndex[f]; ok {
				return idx, true
			}
		}
	}
	return -1, false
}

// Section groups columns for navigation in the UI.
type Section struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Columns     []string `json:"columns"`
}

// OutcomeDef describes an event to analyse: a level of a column, or a
// numeric threshold (Min inclusive, Max inclusive).
type OutcomeDef struct {
	Column string   `json:"column"`
	Equals []string `json:"equals,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// OutcomePreset is a named outcome offered to users and to the assistant.
type OutcomePreset struct {
	ID          string     `json:"id"`
	Label       string     `json:"label"`
	Description string     `json:"description,omitempty"`
	Outcome     OutcomeDef `json:"outcome"`
}

// SourceInfo describes where the data came from.
type SourceInfo struct {
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
	GeneratedAt string `json:"generatedAt"`
}
