package query

import (
	"math"
	"sort"

	"glucolens/internal/dataset"
)

// z95 is the two-sided 95% normal quantile.
const z95 = 1.959963984540054

// Summary describes a set of numbers.
type Summary struct {
	N        int     `json:"n"`
	Mean     float64 `json:"mean"`
	Std      float64 `json:"std"`
	Min      float64 `json:"min"`
	P5       float64 `json:"p5"`
	P25      float64 `json:"p25"`
	Median   float64 `json:"median"`
	P75      float64 `json:"p75"`
	P95      float64 `json:"p95"`
	Max      float64 `json:"max"`
	MeanLow  float64 `json:"meanLow"`
	MeanHigh float64 `json:"meanHigh"`
}

// summarize sorts vals in place and describes them. It returns nil for an
// empty slice.
func summarize(vals []float64) *Summary {
	n := len(vals)
	if n == 0 {
		return nil
	}
	sort.Float64s(vals)
	sum := 0.0
	for _, v := range vals {
		sum += v
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
	half := 0.0
	if n > 1 {
		half = z95 * std / math.Sqrt(float64(n))
	}
	return &Summary{
		N:        n,
		Mean:     finite(mean),
		Std:      finite(std),
		Min:      vals[0],
		P5:       dataset.Quantile(vals, 0.05),
		P25:      dataset.Quantile(vals, 0.25),
		Median:   dataset.Quantile(vals, 0.5),
		P75:      dataset.Quantile(vals, 0.75),
		P95:      dataset.Quantile(vals, 0.95),
		Max:      vals[n-1],
		MeanLow:  finite(mean - half),
		MeanHigh: finite(mean + half),
	}
}

// Rate is a proportion with its Wilson 95% confidence interval.
type Rate struct {
	Events int     `json:"events"`
	N      int     `json:"n"`
	Value  float64 `json:"value"`
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
}

func newRate(events, n int) Rate {
	if n <= 0 {
		return Rate{}
	}
	p := float64(events) / float64(n)
	z2 := z95 * z95
	nf := float64(n)
	denom := 1 + z2/nf
	centre := (p + z2/(2*nf)) / denom
	half := z95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / denom
	return Rate{
		Events: events,
		N:      n,
		Value:  p,
		Low:    math.Max(0, centre-half),
		High:   math.Min(1, centre+half),
	}
}

// Ratio is a ratio estimate with a 95% confidence interval.
type Ratio struct {
	Value float64 `json:"value"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// relativeRisk compares the event rate a/n1 with the reference rate c/n0
// using the log method, with a 0.5 correction when a cell is empty.
func relativeRisk(a, n1, c, n0 int) *Ratio {
	if n1 <= 0 || n0 <= 0 {
		return nil
	}
	fa, fn1, fc, fn0 := float64(a), float64(n1), float64(c), float64(n0)
	if a == 0 || c == 0 || a == n1 || c == n0 {
		fa, fc = fa+0.5, fc+0.5
		fn1, fn0 = fn1+1, fn0+1
	}
	rr := (fa / fn1) / (fc / fn0)
	se := math.Sqrt(1/fa - 1/fn1 + 1/fc - 1/fn0)
	if math.IsNaN(se) || math.IsInf(rr, 0) || rr <= 0 {
		return nil
	}
	return &Ratio{
		Value: rr,
		Low:   math.Exp(math.Log(rr) - z95*se),
		High:  math.Exp(math.Log(rr) + z95*se),
	}
}

// chiSquare runs Pearson's chi-square test of independence on a table of
// counts. Empty rows and columns are ignored.
func chiSquare(table [][]float64) (chi2 float64, df int, n float64) {
	rows := len(table)
	if rows == 0 {
		return 0, 0, 0
	}
	cols := len(table[0])
	rowTot := make([]float64, rows)
	colTot := make([]float64, cols)
	for i, r := range table {
		for j, v := range r {
			rowTot[i] += v
			colTot[j] += v
			n += v
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	usedRows, usedCols := 0, 0
	for _, t := range rowTot {
		if t > 0 {
			usedRows++
		}
	}
	for _, t := range colTot {
		if t > 0 {
			usedCols++
		}
	}
	for i := range table {
		for j := range table[i] {
			exp := rowTot[i] * colTot[j] / n
			if exp > 0 {
				d := table[i][j] - exp
				chi2 += d * d / exp
			}
		}
	}
	df = (usedRows - 1) * (usedCols - 1)
	return chi2, df, n
}

// chiSquarePValue is the upper-tail probability of the chi-square
// distribution.
func chiSquarePValue(chi2 float64, df int) float64 {
	if df <= 0 {
		return 1
	}
	if chi2 <= 0 {
		return 1
	}
	return gammaQ(float64(df)/2, chi2/2)
}

// fPValue is the upper-tail probability of the F distribution.
func fPValue(f float64, d1, d2 int) float64 {
	if d1 <= 0 || d2 <= 0 || f <= 0 || math.IsNaN(f) {
		return 1
	}
	if math.IsInf(f, 1) {
		return 0
	}
	x := float64(d2) / (float64(d2) + float64(d1)*f)
	return betaInc(float64(d2)/2, float64(d1)/2, x)
}

// gammaQ is the regularized upper incomplete gamma function Q(a, x).
func gammaQ(a, x float64) float64 {
	if x < 0 || a <= 0 {
		return 1
	}
	if x == 0 {
		return 1
	}
	lg, _ := math.Lgamma(a)
	if x < a+1 {
		// Series for P(a, x), then Q = 1 - P.
		ap, sum := a, 1/a
		del := sum
		for n := 0; n < 1000; n++ {
			ap++
			del *= x / ap
			sum += del
			if math.Abs(del) < math.Abs(sum)*1e-15 {
				break
			}
		}
		p := sum * math.Exp(-x+a*math.Log(x)-lg)
		return clamp01(1 - p)
	}
	// Continued fraction for Q(a, x) (modified Lentz).
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 1000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return clamp01(math.Exp(-x+a*math.Log(x)-lg) * h)
}

// betaInc is the regularized incomplete beta function I_x(a, b).
func betaInc(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	la, _ := math.Lgamma(a + b)
	lb, _ := math.Lgamma(a)
	lc, _ := math.Lgamma(b)
	front := math.Exp(la - lb - lc + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return clamp01(front * betaCF(a, b, x) / a)
	}
	return clamp01(1 - front*betaCF(b, a, 1-x)/b)
}

func betaCF(a, b, x float64) float64 {
	const tiny = 1e-300
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1-qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m < 1000; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return h
}

func clamp01(f float64) float64 {
	switch {
	case math.IsNaN(f):
		return 1
	case f < 0:
		return 0
	case f > 1:
		return 1
	}
	return f
}

// Strength labels for effect sizes (Cramér's V or the correlation ratio η).
const (
	StrengthNone     = "none"
	StrengthWeak     = "weak"
	StrengthModerate = "moderate"
	StrengthStrong   = "strong"
)

// strengthOf turns an effect size and p-value into a plain label.
func strengthOf(effect, pValue float64) string {
	switch {
	case pValue >= 0.05 || effect < 0.05:
		return StrengthNone
	case effect < 0.10:
		return StrengthWeak
	case effect < 0.25:
		return StrengthModerate
	default:
		return StrengthStrong
	}
}

// anova runs a one-way ANOVA over groups of values and returns the F
// statistic, its p-value and the correlation ratio η (sqrt of η²).
func anova(groups [][]float64) (f, p, eta float64, d1, d2 int) {
	total, n := 0.0, 0
	k := 0
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		k++
		for _, v := range g {
			total += v
		}
		n += len(g)
	}
	if k < 2 || n <= k {
		return 0, 1, 0, 0, 0
	}
	grand := total / float64(n)
	ssb, ssw := 0.0, 0.0
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		m := 0.0
		for _, v := range g {
			m += v
		}
		m /= float64(len(g))
		ssb += float64(len(g)) * (m - grand) * (m - grand)
		for _, v := range g {
			ssw += (v - m) * (v - m)
		}
	}
	d1, d2 = k-1, n-k
	sst := ssb + ssw
	if sst > 0 {
		eta = math.Sqrt(ssb / sst)
	}
	if ssw == 0 {
		if ssb == 0 {
			return 0, 1, 0, d1, d2
		}
		return math.Inf(1), 0, eta, d1, d2
	}
	f = (ssb / float64(d1)) / (ssw / float64(d2))
	return f, fPValue(f, d1, d2), eta, d1, d2
}
