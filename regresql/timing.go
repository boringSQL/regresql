package regresql

import (
	"math"
	"math/rand"
	"sort"
)

// Timing is inspired by ClickHouse: run both engines interleaved, then derive a
// per-query noise threshold by randomly re-splitting the pooled samples. Only a
// difference that beats that threshold counts as a real change.
// https://clickhouse.com/blog/testing-the-performance-of-click-house

const (
	timingMinEffect = 0.05 // ignore diffs under 5%
	timingUnstable  = 0.10 // noise this big and we cant judge
	timingPermIters = 2000
)

type (
	// advisory wall-clock compare for one query
	TimingResult struct {
		BaseMedian   float64 `json:"base_median_ms"`
		TargetMedian float64 `json:"target_median_ms"`
		Ratio        float64 `json:"ratio"` // target / base
		Threshold    float64 `json:"noise_threshold"`
		Status       string  `json:"status"` // slower | faster | noise | unstable
	}
)

// timingVerdict: a real change must beat max(T, 5%), T being the noise
// threshold from the permutation test. T over 10% means unstable.
func timingVerdict(base, target []float64) TimingResult {
	mb, mt := medianOf(base), medianOf(target)
	r := TimingResult{BaseMedian: mb, TargetMedian: mt}

	if len(base) < 3 || len(target) < 3 || mb <= 0 {
		r.Status = "noise" // not enough to say anythign
		return r
	}

	r.Ratio = mt / mb
	r.Threshold = permutationThreshold(base, target, mb)
	observed := math.Abs(mt-mb) / mb

	switch {
	case observed >= math.Max(r.Threshold, timingMinEffect):
		// beats the noise, real change even on a noisy box
		if mt > mb {
			r.Status = "slower"
		} else {
			r.Status = "faster"
		}
	case r.Threshold >= timingUnstable:
		r.Status = "unstable" // under the noise, and noise is big
	default:
		r.Status = "noise"
	}
	return r
}

// permutationThreshold: 95th pct relative median diff over random re-splits of
// the pooled samples. fixed seed so its reproducible
func permutationThreshold(base, target []float64, baseMedian float64) float64 {
	pooled := append(append([]float64{}, base...), target...)
	n := len(base)
	rng := rand.New(rand.NewSource(1))

	diffs := make([]float64, timingPermIters)
	for i := range diffs {
		rng.Shuffle(len(pooled), func(a, b int) { pooled[a], pooled[b] = pooled[b], pooled[a] })
		diffs[i] = math.Abs(medianOf(pooled[:n])-medianOf(pooled[n:])) / baseMedian
	}
	sort.Float64s(diffs)
	return diffs[int(0.95*float64(len(diffs)))]
}

func medianOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}
