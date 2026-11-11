package query

import "math/rand/v2"

// RecordsResponse is one page of rows.
type RecordsResponse struct {
	Total  int              `json:"total"`
	Offset int              `json:"offset"`
	Limit  int              `json:"limit"`
	Rows   []map[string]any `json:"rows"`
}

// Records returns a page of rows matching the filters.
func (e *Engine) Records(filters []Filter, offset, limit int) (*RecordsResponse, error) {
	rows, err := e.Select(filters)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	offset = max(offset, 0)
	resp := &RecordsResponse{Total: len(rows), Offset: offset, Limit: limit, Rows: []map[string]any{}}
	for k := offset; k < len(rows) && k < offset+limit; k++ {
		resp.Rows = append(resp.Rows, e.ds.Row(int(rows[k])))
	}
	return resp, nil
}

// RandomRecord returns one row chosen at random, as a sample patient.
func (e *Engine) RandomRecord() (int, map[string]any) {
	i := rand.IntN(e.ds.Rows)
	return i, e.ds.Row(i)
}
