package strategy

import (
	"encoding/json"
	"math"
	"os"
	"sync"
)

// BayesianPosterior tracks the running Beta distribution of win rate.
// Starts with Beta(83, 3) from Feb 10-12 backtest: 82W/2L.
// Each night, update with new trades: Beta(wins+83, losses+3).
type BayesianPosterior struct {
	Alpha int64 // Beta shape parameter: prior wins + observed wins
	Beta  int64 // Beta shape parameter: prior losses + observed losses
	mu    sync.Mutex
}

// NewBayesianPosterior initializes with Beta(83, 3) prior (82W/2L backtest).
func NewBayesianPosterior() *BayesianPosterior {
	return &BayesianPosterior{
		Alpha: 83,
		Beta:  3,
	}
}

// LoadFromFile reads posterior from disk, or returns default if file doesn't exist.
func (bp *BayesianPosterior) LoadFromFile(path string) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist; use default prior
		return nil
	}

	var stored struct {
		Alpha int64 `json:"alpha"`
		Beta  int64 `json:"beta"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}

	bp.Alpha = stored.Alpha
	bp.Beta = stored.Beta
	return nil
}

// SaveToFile writes posterior to disk.
func (bp *BayesianPosterior) SaveToFile(path string) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	data := struct {
		Alpha int64 `json:"alpha"`
		Beta  int64 `json:"beta"`
	}{
		Alpha: bp.Alpha,
		Beta:  bp.Beta,
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, bytes, 0644)
}

// UpdateWithTrades adds observed wins/losses to the posterior.
// Call this nightly with (wins, losses) from the day's trading.
func (bp *BayesianPosterior) UpdateWithTrades(wins, losses int64) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.Alpha += wins
	bp.Beta += losses
}

// Mean returns the posterior mean (point estimate).
func (bp *BayesianPosterior) Mean() float64 {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.Alpha+bp.Beta == 0 {
		return 0.5
	}
	return float64(bp.Alpha) / float64(bp.Alpha+bp.Beta)
}

// Median returns the posterior median (robust to asymmetry).
// For Beta distribution, approximation: median ≈ (α - 1/3) / (α + β - 2/3)
// For large α, β: median ≈ α / (α + β) - 1/(6(α+β))
func (bp *BayesianPosterior) Median() float64 {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.Alpha+bp.Beta == 0 {
		return 0.5
	}

	// Use exact Beta quantile approximation
	// For Beta(a, b), median ≈ (a - 1/3) / (a + b - 2/3)
	// This is accurate for moderate to large parameters
	a := float64(bp.Alpha)
	b := float64(bp.Beta)

	if a > 1 && b > 1 {
		// Accurate approximation for a, b > 1
		return (a - 1/3) / (a + b - 2/3)
	}

	// Fallback to mean for edge cases
	return a / (a + b)
}

// CredibleInterval returns the [lower, upper] Bayesian credible interval.
// Uses simple quantile approximation based on normal approximation to Beta.
func (bp *BayesianPosterior) CredibleInterval(confidence float64) [2]float64 {
	bp.mu.Lock()
	a := float64(bp.Alpha)
	b := float64(bp.Beta)
	bp.mu.Unlock()

	if a+b == 0 {
		return [2]float64{0, 1}
	}

	mean := a / (a + b)
	// Beta variance: α*β / ((α+β)²*(α+β+1))
	variance := (a * b) / ((a + b) * (a + b) * (a + b + 1))
	sd := math.Sqrt(variance)

	// Normal approximation; for 95% use z ≈ 1.96
	z := 1.96 * (1 - confidence) / 2 // Adjust for confidence level
	if z < 0 {
		z = -z
	}

	lower := math.Max(0, mean-z*sd)
	upper := math.Min(1, mean+z*sd)

	return [2]float64{lower, upper}
}

// String returns a human-readable summary.
func (bp *BayesianPosterior) String() string {
	return "Beta(" + string(rune(bp.Alpha)) + ", " + string(rune(bp.Beta)) + ")"
}
