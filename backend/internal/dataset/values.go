package dataset

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Value is one patient value, checked against and normalised for a column.
type Value struct {
	Column  *Column
	Num     float64 // numeric value; NaN for categorical columns
	Level   int     // level index; -1 when the value is not one of the levels
	Display string  // human-readable, with unit
}

// JSON returns the value in the same shape Dataset.Row uses.
func (v Value) JSON() any {
	if v.Column.IsNumeric() {
		return v.Num
	}
	return v.Column.Levels[v.Level]
}

// Normalize checks a raw value (number, numeric string, label, bool) against
// the column and returns it in canonical form.
func (c *Column) Normalize(raw any) (Value, error) {
	val := Value{Column: c, Num: math.NaN(), Level: -1}
	switch c.Kind {
	case KindText:
		return val, fmt.Errorf("%s is free text and cannot be compared", c.Label)

	case KindCategorical:
		s, ok := asString(raw)
		if !ok {
			return val, fmt.Errorf("%s needs one of: %s", c.Label, strings.Join(c.Levels, ", "))
		}
		idx, found := c.FindLevel(s)
		if !found {
			return val, fmt.Errorf("%q is not a value of %s (valid: %s)", s, c.Label, strings.Join(c.Levels, ", "))
		}
		val.Level = idx
		val.Display = c.Levels[idx]
		return val, nil

	case KindBinary:
		if b, ok := raw.(bool); ok {
			raw = map[bool]string{true: "1", false: "0"}[b]
		}
		s, ok := asString(raw)
		if !ok {
			return val, fmt.Errorf("%s needs %s or %s", c.Label, c.Levels[0], c.Levels[1])
		}
		idx, found := c.FindLevel(s)
		if !found {
			return val, fmt.Errorf("%q is not a value of %s (use %s or %s)", s, c.Label, c.Levels[0], c.Levels[1])
		}
		val.Level = idx
		val.Num = c.levelValues[idx]
		val.Display = c.Levels[idx]
		return val, nil
	}

	f, ok := asFloat(raw)
	if !ok {
		return val, fmt.Errorf("%s needs a number", c.Label)
	}
	val.Num = f
	if c.Kind == KindDiscrete {
		if idx, found := c.levelIndex[f]; found {
			val.Level = idx
		}
	}
	val.Display = c.FormatValue(f)
	return val, nil
}

// FormatValue renders a numeric value with the column's precision and unit.
func (c *Column) FormatValue(f float64) string {
	s := FormatNumber(f, c.Decimals)
	switch {
	case c.Unit == "":
		return s
	case c.Unit == "%" || strings.HasPrefix(c.Unit, "/"):
		return s + c.Unit
	default:
		return s + " " + c.Unit
	}
}

// FormatNumber renders f with up to the given number of decimals, trimming
// trailing zeros.
func FormatNumber(f float64, decimals int) string {
	if math.IsNaN(f) {
		return "–"
	}
	s := strconv.FormatFloat(f, 'f', decimals, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// GeneralBands returns the bands that apply to everyone.
func (c *Column) GeneralBands() []Band {
	var out []Band
	for _, b := range c.Bands {
		if b.When == nil {
			out = append(out, b)
		}
	}
	return out
}

// BandsFor returns the bands that apply to a person: condition-specific bands
// (for example by gender) when the person's values meet the condition,
// otherwise the general bands.
func (c *Column) BandsFor(person map[string]Value) []Band {
	var specific []Band
	for _, b := range c.Bands {
		if b.When == nil {
			continue
		}
		if v, ok := person[b.When.Column]; ok && v.Level >= 0 && v.Column.HasLevels() &&
			strings.EqualFold(v.Column.Levels[v.Level], b.When.Equals) {
			specific = append(specific, b)
		}
	}
	if len(specific) > 0 {
		return specific
	}
	return c.GeneralBands()
}

// BandOf returns the band containing v, chosen from BandsFor(person).
func (c *Column) BandOf(v float64, person map[string]Value) *Band {
	for _, b := range c.BandsFor(person) {
		if b.Contains(v) {
			b := b
			return &b
		}
	}
	return nil
}

func asString(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v), true
	case json.Number:
		return v.String(), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	}
	return "", false
}

func asFloat(raw any) (float64, bool) {
	var f float64
	switch v := raw.(type) {
	case float64:
		f = v
	case float32:
		f = float64(v)
	case int:
		f = float64(v)
	case int64:
		f = float64(v)
	case int32:
		f = float64(v)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		f = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// NormalizeAll checks a map of raw patient values. Unknown columns and
// invalid values are reported per column; valid values are returned.
func (d *Dataset) NormalizeAll(raw map[string]any) (map[string]Value, map[string]string) {
	out := map[string]Value{}
	errs := map[string]string{}
	for name, rv := range raw {
		if rv == nil {
			continue
		}
		if s, ok := rv.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		c, ok := d.Column(name)
		if !ok {
			errs[name] = "unknown column"
			continue
		}
		v, err := c.Normalize(rv)
		if err != nil {
			errs[name] = err.Error()
			continue
		}
		out[name] = v
	}
	return out, errs
}

// LowerFirst lowercases the first letter for use mid-sentence, unless the
// first word looks like an acronym or a lab name ("BMI", "HbA1c", "HDL").
func LowerFirst(s string) string {
	if s == "" {
		return s
	}
	word := s
	if i := strings.IndexByte(s, ' '); i >= 0 {
		word = s[:i]
	}
	for _, r := range word[1:] {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return s
		}
	}
	return strings.ToLower(s[:1]) + s[1:]
}
