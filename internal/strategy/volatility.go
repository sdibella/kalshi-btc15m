package strategy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"
)

// VolFilter blocks trading when BTC price volatility is too high.
// Reads BRTI price from the data collector's JSONL file on the same VPS.
// Computes rolling standard deviation over a configurable window.
// Defaults to safe (allows trading) if data is unavailable.
type VolFilter struct {
	mu        sync.Mutex
	samples   []priceSample
	window    time.Duration
	maxStdDev float64 // in dollars — block trading if stddev exceeds this

	// Data collector file reading
	dataDir        string // path to data collector data directory
	lastRead       time.Time
	lastFileOffset int64
	lastFileName   string
}

type priceSample struct {
	Price float64
	Time  time.Time
}

// NewVolFilter creates a volatility filter.
// dataDir: path to data collector's data directory (e.g., /home/stefan/KalshiBTC15min-data/data)
// window: rolling window duration (e.g., 15 minutes)
// maxStdDev: stddev threshold in dollars to block trading (e.g., 200.0)
func NewVolFilter(dataDir string, window time.Duration, maxStdDev float64) *VolFilter {
	return &VolFilter{
		dataDir:   dataDir,
		window:    window,
		maxStdDev: maxStdDev,
	}
}

// Update reads the latest BTC price from the data collector's JSONL file.
// Call this periodically (e.g., every 10 seconds) from the engine tick.
// Returns the latest BRTI price, or 0 if unavailable.
func (v *VolFilter) Update() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now().UTC()
	fileName := fmt.Sprintf("%s/kxbtc15m-%s.jsonl", v.dataDir, now.Format("2006-01-02"))

	// Read new lines from the file
	price, ts := v.readLatestPrice(fileName)
	if price <= 0 {
		return 0
	}

	// Add sample and trim old ones
	v.samples = append(v.samples, priceSample{Price: price, Time: ts})
	v.trimOldSamples(now)

	return price
}

// readLatestPrice reads the last line of the JSONL file to get the most recent BRTI price.
// Uses seek-from-end for efficiency on large files.
func (v *VolFilter) readLatestPrice(fileName string) (float64, time.Time) {
	f, err := os.Open(fileName)
	if err != nil {
		return 0, time.Time{}
	}
	defer f.Close()

	// Seek to end minus a buffer to find the last complete line
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return 0, time.Time{}
	}

	// Read last 4KB — more than enough for one JSONL line
	bufSize := int64(4096)
	offset := info.Size() - bufSize
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, time.Time{}
	}

	// Find the last complete line
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	var lastLine string
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			lastLine = line
		}
	}

	if lastLine == "" {
		return 0, time.Time{}
	}

	// Parse the JSONL tick
	var tick struct {
		Type string  `json:"type"`
		Ts   string  `json:"ts"`
		BRTI float64 `json:"brti"`
	}
	if err := json.Unmarshal([]byte(lastLine), &tick); err != nil {
		return 0, time.Time{}
	}

	if tick.BRTI <= 0 {
		return 0, time.Time{}
	}

	ts, err := time.Parse(time.RFC3339Nano, tick.Ts)
	if err != nil {
		ts = time.Now().UTC()
	}

	return tick.BRTI, ts
}

func (v *VolFilter) trimOldSamples(now time.Time) {
	cutoff := now.Add(-v.window)
	i := 0
	for i < len(v.samples) && v.samples[i].Time.Before(cutoff) {
		i++
	}
	if i > 0 {
		v.samples = v.samples[i:]
	}
}

// StdDev returns the rolling standard deviation of BTC price in dollars.
// Returns 0 if insufficient data (< 2 samples).
func (v *VolFilter) StdDev() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stddevLocked()
}

func (v *VolFilter) stddevLocked() float64 {
	if len(v.samples) < 2 {
		return 0
	}

	var sum float64
	for _, s := range v.samples {
		sum += s.Price
	}
	mean := sum / float64(len(v.samples))

	var variance float64
	for _, s := range v.samples {
		diff := s.Price - mean
		variance += diff * diff
	}
	variance /= float64(len(v.samples) - 1) // sample variance

	return math.Sqrt(variance)
}

// IsSafe returns true if volatility is below the threshold (OK to trade).
// Returns true (safe) if no data is available — vol filter is a bonus, not a gate.
func (v *VolFilter) IsSafe() bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.samples) < 2 {
		return true // no data → allow trading
	}

	return v.stddevLocked() <= v.maxStdDev
}

// SampleCount returns the number of price samples in the rolling window.
func (v *VolFilter) SampleCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.samples)
}
