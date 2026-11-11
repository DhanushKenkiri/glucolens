// Package query answers analytical questions over the in-memory dataset:
// distributions, group comparisons, risk analysis, percentiles, similar
// people, correlations and relationships. The REST API and the assistant's
// tools both call this package, so charts and answers always agree.
package query

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"glucolens/internal/dataset"
)

// MinGroupSize is the smallest group whose figures are considered reliable.
const MinGroupSize = 30

// maxGroups limits how many groups a split or comparison may produce.
const maxGroups = 12

// Engine runs queries against one dataset.
type Engine struct {
	ds      *dataset.Dataset
	allRows []int32

	corrOnce sync.Once
	corr     *CorrelationResponse
}

// New creates an engine for the dataset.
func New(ds *dataset.Dataset) *Engine {
	all := make([]int32, ds.Rows)
	for i := range all {
		all[i] = int32(i)
	}
	return &Engine{ds: ds, allRows: all}
}

// Dataset returns the underlying dataset.
func (e *Engine) Dataset() *dataset.Dataset { return e.ds }

// ValidationError marks a problem with the request rather than the server.
type ValidationError struct{ Message string }

func (v *ValidationError) Error() string { return v.Message }

func invalidf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// IsValidation reports whether err is a ValidationError.
func IsValidation(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// column looks up an analysable column or returns a helpful error.
func (e *Engine) column(name, role string) (*dataset.Column, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, invalidf("%s is required", role)
	}
	c, ok := e.ds.Column(name)
	if !ok {
		return nil, invalidf("unknown %s %q; valid columns: %s", role, name, strings.Join(e.ds.ColumnNames(), ", "))
	}
	if !c.Analysable() {
		return nil, invalidf("column %q is free text and cannot be analysed", name)
	}
	return c, nil
}

// SchemaResponse describes the dataset for clients.
type SchemaResponse struct {
	Dataset      DatasetInfo             `json:"dataset"`
	Sections     []dataset.Section       `json:"sections"`
	Columns      []*dataset.Column       `json:"columns"`
	Outcomes     []dataset.OutcomePreset `json:"outcomes"`
	MinGroupSize int                     `json:"minGroupSize"`
}

// DatasetInfo is the dataset-level part of the schema.
type DatasetInfo struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Rows        int                `json:"rows"`
	Source      dataset.SourceInfo `json:"source"`
}

// Schema returns the dataset description.
func (e *Engine) Schema() SchemaResponse {
	return SchemaResponse{
		Dataset: DatasetInfo{
			Name:        e.ds.Name,
			Description: e.ds.Description,
			Rows:        e.ds.Rows,
			Source:      e.ds.Source,
		},
		Sections:     e.ds.Sections,
		Columns:      e.ds.Columns,
		Outcomes:     e.ds.Outcomes,
		MinGroupSize: MinGroupSize,
	}
}

// finite replaces NaN and infinities (which JSON cannot carry) with 0.
func finite(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// round keeps responses compact and readable.
func round(f float64, decimals int) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	p := math.Pow(10, float64(decimals))
	return math.Round(f*p) / p
}

func ptr(f float64) *float64 {
	f = finite(f)
	return &f
}

func pct(f float64) string {
	return strconv.FormatFloat(f*100, 'f', 1, 64) + "%"
}

// lowerFirst lowercases a label for use mid-sentence (acronyms stay).
func lowerFirst(s string) string { return dataset.LowerFirst(s) }

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
