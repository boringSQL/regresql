package regresql

import "testing"

// Tight samples with a clear 50% gap: the target is really slower, and the small
// run-to-run spread means the permutation noise can't explain the gap.
func TestTimingVerdict_Slower(t *testing.T) {
	base := []float64{100, 101, 99, 100, 102, 98, 101, 100, 99, 100}
	target := []float64{150, 151, 149, 150, 152, 148, 151, 150, 149, 150}

	v := timingVerdict(base, target)
	if v.Status != "slower" {
		t.Errorf("status = %q (ratio %.2f, T %.2f), want slower", v.Status, v.Ratio, v.Threshold)
	}
	if v.Ratio < 1.4 {
		t.Errorf("ratio = %.2f, want ~1.5", v.Ratio)
	}
}

// The same gap the other way round reads as faster.
func TestTimingVerdict_Faster(t *testing.T) {
	base := []float64{150, 151, 149, 150, 152, 148, 151, 150, 149, 150}
	target := []float64{100, 101, 99, 100, 102, 98, 101, 100, 99, 100}

	if v := timingVerdict(base, target); v.Status != "faster" {
		t.Errorf("status = %q, want faster", v.Status)
	}
}

// A 1% difference between two tight runs is under the 5% floor — noise, not a change.
func TestTimingVerdict_Noise(t *testing.T) {
	base := []float64{100, 101, 99, 100, 102, 98, 101, 100, 99, 100}
	target := []float64{101, 102, 100, 101, 103, 99, 102, 101, 100, 101}

	if v := timingVerdict(base, target); v.Status != "noise" {
		t.Errorf("status = %q, want noise", v.Status)
	}
}

// A small shift buried in a very jumpy run: the noise alone can move the median
// more than 10% and the shift doesn't beat it, so we can't tell — unstable.
func TestTimingVerdict_Unstable(t *testing.T) {
	base := []float64{60, 140, 80, 120, 100, 70, 130, 90, 110, 100}
	target := []float64{63, 147, 84, 126, 105, 74, 137, 95, 116, 105}

	if v := timingVerdict(base, target); v.Status != "unstable" {
		t.Errorf("status = %q (T %.2f), want unstable", v.Status, v.Threshold)
	}
}

// A big, clean slowdown must still be reported even when the samples are noisy —
// this is the case that a "check unstable first" ordering got wrong.
func TestTimingVerdict_BigChangeBeatsNoise(t *testing.T) {
	base := []float64{90, 110, 95, 105, 100, 92, 108, 98, 102, 100}
	target := []float64{270, 330, 285, 315, 300, 276, 324, 294, 306, 300}

	if v := timingVerdict(base, target); v.Status != "slower" {
		t.Errorf("status = %q (ratio %.2f, T %.2f), want slower", v.Status, v.Ratio, v.Threshold)
	}
}

// Too few samples to say anything.
func TestTimingVerdict_TooFewSamples(t *testing.T) {
	if v := timingVerdict([]float64{100, 200}, []float64{300, 400}); v.Status != "noise" {
		t.Errorf("status = %q, want noise", v.Status)
	}
}

// The permutation threshold uses a fixed seed, so the same samples always give
// the same threshold — no flaky verdicts run to run.
func TestPermutationThreshold_Deterministic(t *testing.T) {
	base := []float64{100, 110, 95, 105, 100}
	target := []float64{130, 140, 125, 135, 130}
	m := medianOf(base)

	if a, b := permutationThreshold(base, target, m), permutationThreshold(base, target, m); a != b {
		t.Errorf("threshold not deterministic: %.4f vs %.4f", a, b)
	}
}

func TestMedianOf(t *testing.T) {
	if m := medianOf([]float64{3, 1, 2}); m != 2 {
		t.Errorf("odd median = %v, want 2", m)
	}
	if m := medianOf([]float64{4, 1, 3, 2}); m != 2.5 {
		t.Errorf("even median = %v, want 2.5", m)
	}
	if m := medianOf(nil); m != 0 {
		t.Errorf("empty median = %v, want 0", m)
	}
}
