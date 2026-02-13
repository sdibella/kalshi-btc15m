package strategy

import (
	"testing"
	"time"
)

func TestVolFilterStdDev(t *testing.T) {
	vf := NewVolFilter("/nonexistent", 15*time.Minute, 200.0)

	// No samples → stddev = 0
	if got := vf.StdDev(); got != 0 {
		t.Errorf("empty StdDev() = %f, want 0", got)
	}

	// Add constant prices → stddev = 0
	now := time.Now()
	vf.mu.Lock()
	for i := 0; i < 10; i++ {
		vf.samples = append(vf.samples, priceSample{
			Price: 66000.0,
			Time:  now.Add(time.Duration(i) * time.Second),
		})
	}
	vf.mu.Unlock()

	if got := vf.StdDev(); got != 0 {
		t.Errorf("constant prices StdDev() = %f, want 0", got)
	}

	// Add varying prices
	vf.mu.Lock()
	vf.samples = nil
	prices := []float64{66000, 66100, 66200, 66300, 66400}
	for i, p := range prices {
		vf.samples = append(vf.samples, priceSample{
			Price: p,
			Time:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	vf.mu.Unlock()

	got := vf.StdDev()
	// stddev of [66000, 66100, 66200, 66300, 66400] = 158.11 (sample stddev)
	if got < 155 || got > 162 {
		t.Errorf("varying prices StdDev() = %f, want ~158.11", got)
	}
}

func TestVolFilterIsSafe(t *testing.T) {
	tests := []struct {
		name      string
		prices    []float64
		maxStdDev float64
		want      bool
	}{
		{
			name:      "no samples → safe",
			prices:    nil,
			maxStdDev: 200,
			want:      true,
		},
		{
			name:      "one sample → safe",
			prices:    []float64{66000},
			maxStdDev: 200,
			want:      true,
		},
		{
			name:      "calm market → safe",
			prices:    []float64{66000, 66010, 65990, 66005, 66015},
			maxStdDev: 200,
			want:      true, // stddev ≈ 10
		},
		{
			name:      "volatile market → unsafe",
			prices:    []float64{66000, 66500, 65500, 67000, 65000},
			maxStdDev: 200,
			want:      false, // stddev ≈ 791
		},
		{
			name:      "at threshold → safe",
			prices:    []float64{66000, 66100, 66200, 66300, 66400},
			maxStdDev: 200,
			want:      true, // stddev ≈ 158, below 200
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vf := NewVolFilter("/nonexistent", 15*time.Minute, tt.maxStdDev)

			now := time.Now()
			vf.mu.Lock()
			for i, p := range tt.prices {
				vf.samples = append(vf.samples, priceSample{
					Price: p,
					Time:  now.Add(time.Duration(i) * time.Minute),
				})
			}
			vf.mu.Unlock()

			if got := vf.IsSafe(); got != tt.want {
				t.Errorf("IsSafe() = %v, want %v (stddev=%.2f)", got, tt.want, vf.StdDev())
			}
		})
	}
}

func TestVolFilterTrimOldSamples(t *testing.T) {
	vf := NewVolFilter("/nonexistent", 15*time.Minute, 200.0)

	now := time.Now()
	vf.mu.Lock()
	// Add samples: some old (20min ago), some recent (5min ago)
	vf.samples = []priceSample{
		{Price: 66000, Time: now.Add(-20 * time.Minute)},
		{Price: 66100, Time: now.Add(-18 * time.Minute)},
		{Price: 66200, Time: now.Add(-5 * time.Minute)},
		{Price: 66300, Time: now.Add(-2 * time.Minute)},
		{Price: 66400, Time: now.Add(-1 * time.Minute)},
	}
	vf.trimOldSamples(now)
	vf.mu.Unlock()

	if got := vf.SampleCount(); got != 3 {
		t.Errorf("after trim, SampleCount() = %d, want 3", got)
	}
}
