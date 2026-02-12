package strategy

import (
	"testing"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		yesBid     int
		yesAsk     int
		wantSide   string
		wantLimit  int
		wantRefAsk int
	}{
		{
			name:       "yes signal: yesAsk=85, buy YES at ask=85",
			yesBid:     82,
			yesAsk:     85,
			wantSide:   "yes",
			wantLimit:  85,
			wantRefAsk: 85,
		},
		{
			name:       "yes signal: yesAsk=80 (exact threshold)",
			yesBid:     77,
			yesAsk:     80,
			wantSide:   "yes",
			wantLimit:  80,
			wantRefAsk: 80,
		},
		{
			name:       "no signal: noAsk=85 (yesBid=15, yesAsk=18)",
			yesBid:     15,
			yesAsk:     18,
			wantSide:   "no",
			wantLimit:  85,
			wantRefAsk: 85,
		},
		{
			name:       "no signal: noAsk=80 (yesBid=20, yesAsk=23)",
			yesBid:     20,
			yesAsk:     23,
			wantSide:   "no",
			wantLimit:  80,
			wantRefAsk: 80,
		},
		{
			name:     "no trade: yesAsk=75 below threshold",
			yesBid:   70,
			yesAsk:   75,
			wantSide: "",
		},
		{
			name:     "no trade: both below 80",
			yesBid:   48,
			yesAsk:   52,
			wantSide: "",
		},
		{
			name:     "no trade: 50/50 market",
			yesBid:   50,
			yesAsk:   50,
			wantSide: "",
		},
		{
			name:       "yes signal takes priority: yesAsk=85, noAsk=85",
			yesBid:     15,
			yesAsk:     85,
			wantSide:   "yes",
			wantLimit:  85,
			wantRefAsk: 85,
		},
		{
			name:       "strong yes: yesAsk=90",
			yesBid:     85,
			yesAsk:     90,
			wantSide:   "yes",
			wantLimit:  90,
			wantRefAsk: 90,
		},
		{
			name:       "strong no: yesBid=10, yesAsk=15",
			yesBid:     10,
			yesAsk:     15,
			wantSide:   "no",
			wantLimit:  90,
			wantRefAsk: 90,
		},
		{
			name:     "no trade: yesAsk=79 just below threshold",
			yesBid:   76,
			yesAsk:   79,
			wantSide: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := Evaluate(tt.yesBid, tt.yesAsk)
			if sig.Side != tt.wantSide {
				t.Errorf("Evaluate(%d, %d).Side = %q, want %q", tt.yesBid, tt.yesAsk, sig.Side, tt.wantSide)
			}
			if tt.wantSide != "" {
				if sig.LimitPrice != tt.wantLimit {
					t.Errorf("Evaluate(%d, %d).LimitPrice = %d, want %d", tt.yesBid, tt.yesAsk, sig.LimitPrice, tt.wantLimit)
				}
				if sig.RefAsk != tt.wantRefAsk {
					t.Errorf("Evaluate(%d, %d).RefAsk = %d, want %d", tt.yesBid, tt.yesAsk, sig.RefAsk, tt.wantRefAsk)
				}
			}
		})
	}
}

func TestInEntryWindow(t *testing.T) {
	// InEntryWindow uses seconds until market CLOSE.
	// Entry window: 0 < secsUntilClose <= 240 (last 4 minutes before close).
	// Per spec: evaluate once when entering this window.
	tests := []struct {
		name     string
		secsLeft float64
		want     bool
	}{
		{"inside: 200s until close", 200, true},
		{"inside: 120s until close", 120, true},
		{"inside: 240s until close (boundary)", 240, true},
		{"inside: 1s until close", 1, true},
		{"outside: 0s until close (boundary)", 0, false},
		{"outside: 241s until close", 241, false},
		{"outside: 300s until close", 300, false},
		{"outside: 540s until close", 540, false},
		{"outside: negative", -10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InEntryWindow(tt.secsLeft)
			if got != tt.want {
				t.Errorf("InEntryWindow(%v) = %v, want %v", tt.secsLeft, got, tt.want)
			}
		})
	}
}

func TestTakerFee(t *testing.T) {
	tests := []struct {
		name       string
		contracts  int
		priceCents int
		want       int
	}{
		{
			name:       "1 contract at 50c",
			contracts:  1,
			priceCents: 50,
			want:       2, // ceil(0.07 * 1 * 0.5 * 0.5 * 100) = ceil(1.75) = 2
		},
		{
			name:       "1 contract at 60c",
			contracts:  1,
			priceCents: 60,
			want:       2, // ceil(0.07 * 1 * 0.6 * 0.4 * 100) = ceil(1.68) = 2
		},
		{
			name:       "1 contract at 90c",
			contracts:  1,
			priceCents: 90,
			want:       1, // ceil(0.07 * 1 * 0.9 * 0.1 * 100) = ceil(0.63) = 1
		},
		{
			name:       "5 contracts at 55c",
			contracts:  5,
			priceCents: 55,
			want:       9, // ceil(0.07 * 5 * 0.55 * 0.45 * 100) = ceil(8.6625) = 9
		},
		{
			name:       "1 contract at 10c",
			contracts:  1,
			priceCents: 10,
			want:       1, // ceil(0.07 * 1 * 0.1 * 0.9 * 100) = ceil(0.63) = 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TakerFee(tt.contracts, tt.priceCents)
			if got != tt.want {
				t.Errorf("TakerFee(%d, %d) = %d, want %d", tt.contracts, tt.priceCents, got, tt.want)
			}
		})
	}
}

func TestKellySize(t *testing.T) {
	// Uses spec formula with AssumedWinRate=0.935:
	//   fee = 0.07 * min(entry, 100-entry)
	//   b = (100 - entry - fee) / (entry + fee)
	//   kelly = p - (q / b)
	//   contracts = floor(0.25 * kelly * balance / (entry + fee))
	tests := []struct {
		name         string
		limitPrice   int
		balanceCents int
		want         int
	}{
		{
			// entry=55, fee=0.07*45=3.15, winProfit=41.85, loss=58.15
			// b=41.85/58.15=0.7196, kelly=0.935-(0.065/0.7196)=0.8447
			// quarter=0.2112, cost=58.15, contracts=floor(0.2112*35537/58.15)=129
			name:         "entry at 55c, bal=$355.37",
			limitPrice:   55,
			balanceCents: 35537,
			want:         129,
		},
		{
			// entry=80, fee=0.07*20=1.40, winProfit=18.60, loss=81.40
			// b=18.60/81.40=0.2285, kelly=0.935-(0.065/0.2285)=0.6506
			// quarter=0.1627, cost=81.40, contracts=floor(0.1627*35537/81.40)=71
			name:         "entry at 80c (typical), bal=$355.37",
			limitPrice:   80,
			balanceCents: 35537,
			want:         71,
		},
		{
			// Same as above but $1000 balance — scales proportionally
			name:         "entry at 80c, bal=$1000",
			limitPrice:   80,
			balanceCents: 100000,
			want:         199,
		},
		{
			name:         "zero balance",
			limitPrice:   55,
			balanceCents: 0,
			want:         0,
		},
		{
			// entry=55, cost=58.15, quarter=0.2112
			// bet=floor(0.2112*500)=105, contracts=105/58.15=1
			name:         "tiny balance: $5",
			limitPrice:   55,
			balanceCents: 500,
			want:         1,
		},
		{
			name:         "invalid price: 0",
			limitPrice:   0,
			balanceCents: 35537,
			want:         0,
		},
		{
			name:         "invalid price: 100",
			limitPrice:   100,
			balanceCents: 35537,
			want:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KellySize(tt.limitPrice, tt.balanceCents)
			if got != tt.want {
				t.Errorf("KellySize(%d, %d) = %d, want %d",
					tt.limitPrice, tt.balanceCents, got, tt.want)
			}
		})
	}
}

func TestComputePnL(t *testing.T) {
	tests := []struct {
		name       string
		won        bool
		entryPrice int
		contracts  int
		feeCents   int
		want       int
	}{
		{
			name:       "win at 55c: (100-55)*1 - 2 = 43",
			won:        true,
			entryPrice: 55,
			contracts:  1,
			feeCents:   2,
			want:       43,
		},
		{
			name:       "loss at 55c: -(55*1 + 2) = -57",
			won:        false,
			entryPrice: 55,
			contracts:  1,
			feeCents:   2,
			want:       -57,
		},
		{
			name:       "win at 40c: (100-40)*1 - 2 = 58",
			won:        true,
			entryPrice: 40,
			contracts:  1,
			feeCents:   2,
			want:       58,
		},
		{
			name:       "loss at 40c: -(40*1 + 2) = -42",
			won:        false,
			entryPrice: 40,
			contracts:  1,
			feeCents:   2,
			want:       -42,
		},
		{
			name:       "win 5 contracts at 60c: (100-60)*5 - 9 = 191",
			won:        true,
			entryPrice: 60,
			contracts:  5,
			feeCents:   9,
			want:       191,
		},
		{
			name:       "loss 5 contracts at 60c: -(60*5 + 9) = -309",
			won:        false,
			entryPrice: 60,
			contracts:  5,
			feeCents:   9,
			want:       -309,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputePnL(tt.won, tt.entryPrice, tt.contracts, tt.feeCents)
			if got != tt.want {
				t.Errorf("ComputePnL(%v, %d, %d, %d) = %d, want %d", tt.won, tt.entryPrice, tt.contracts, tt.feeCents, got, tt.want)
			}
		})
	}
}
