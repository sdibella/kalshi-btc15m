package kalshi

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sdibella/kalshi-btc15m/internal/config"
)

type Client struct {
	cfg            *config.Config
	privKey        *rsa.PrivateKey
	http           *http.Client
	baseURL        string
	basePathPrefix string // e.g. "/trade-api/v2"
}

func NewClient(cfg *config.Config) (*Client, error) {
	key, err := LoadPrivateKey(cfg.KalshiPrivKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading kalshi key: %w", err)
	}

	// Extract the URL path prefix (e.g. "/trade-api/v2") for signing
	parsed, err := url.Parse(cfg.BaseURL())
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	return &Client{
		cfg:            cfg,
		privKey:        key,
		http:           &http.Client{Timeout: 10 * time.Second},
		baseURL:        cfg.BaseURL(),
		basePathPrefix: parsed.Path,
	}, nil
}

// signPath returns the full API path for signature computation.
// e.g. "/portfolio/balance" -> "/trade-api/v2/portfolio/balance"
func (c *Client) signPath(path string) string {
	return c.basePathPrefix + path
}

// --- API Types ---

type Market struct {
	Ticker                 string  `json:"ticker"`
	EventTicker            string  `json:"event_ticker"`
	Title                  string  `json:"title"`
	Status                 string  `json:"status"`
	YesBid                 int     `json:"yes_bid"`
	YesAsk                 int     `json:"yes_ask"`
	NoBid                  int     `json:"no_bid"`
	NoAsk                  int     `json:"no_ask"`
	LastPrice              int     `json:"last_price"`
	Volume                 int     `json:"volume"`
	OpenInterest           int     `json:"open_interest"`
	FloorStrike            float64 `json:"floor_strike"`
	CapStrike              float64 `json:"cap_strike"`
	CloseTime              string  `json:"close_time"`
	OpenTime               string  `json:"open_time"`
	ExpirationTime         string  `json:"expiration_time"`
	ExpectedExpirationTime string  `json:"expected_expiration_time"`
	Result                 string  `json:"result"`
	Subtitle               string  `json:"subtitle"`
	YesSubTitle            string  `json:"yes_sub_title"`
	NoSubTitle             string  `json:"no_sub_title"`
	CustomStrike           string  `json:"custom_strike"`
	RulesPrimary           string  `json:"rules_primary"`
}

func (m *Market) StrikePrice() float64 {
	if m.CapStrike > 0 {
		return m.CapStrike
	}
	if m.FloorStrike > 0 {
		return m.FloorStrike
	}

	// For KXBTC15M markets, parse strike from rules_primary
	// Example: "...is at least 70382.44, then..."
	if m.RulesPrimary != "" {
		var strike float64
		// Look for "at least X" pattern
		if _, err := fmt.Sscanf(m.RulesPrimary, "%*s at least %f", &strike); err == nil && strike > 0 {
			return strike
		}
		// Look for "is at least X" pattern (more common)
		re := regexp.MustCompile(`is at least ([\d.]+)`)
		if matches := re.FindStringSubmatch(m.RulesPrimary); len(matches) > 1 {
			if strike, err := strconv.ParseFloat(matches[1], 64); err == nil {
				return strike
			}
		}
	}

	return 0
}

func (m *Market) ExpirationParsed() (time.Time, error) {
	// Use expected_expiration_time for 15-min markets (actual resolution time)
	if m.ExpectedExpirationTime != "" {
		return time.Parse(time.RFC3339, m.ExpectedExpirationTime)
	}
	return time.Parse(time.RFC3339, m.ExpirationTime)
}

func (m *Market) CloseTimeParsed() (time.Time, error) {
	if m.CloseTime == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, m.CloseTime)
}

type Orderbook struct {
	Ticker string  `json:"ticker"`
	Yes    [][]int `json:"yes"` // [[price, quantity], ...]
	No     [][]int `json:"no"`
}

func (ob *Orderbook) BestYesBid() int {
	if len(ob.Yes) > 0 && len(ob.Yes[0]) >= 2 {
		return ob.Yes[0][0]
	}
	return 0
}

func (ob *Orderbook) BestYesAsk() int {
	if len(ob.No) > 0 && len(ob.No[0]) >= 2 {
		return 100 - ob.No[0][0]
	}
	return 100
}

type Balance struct {
	Balance int `json:"balance"` // cents
}

type Position struct {
	Ticker             string `json:"ticker"`
	MarketExposure     int    `json:"market_exposure"`
	RestingOrdersCount int    `json:"resting_orders_count"`
	TotalTraded        int    `json:"total_traded"`
	RealizedPnl        int    `json:"realized_pnl"`
	Position           int    `json:"position"` // positive=YES, negative=NO
}

type OrderRequest struct {
	Ticker      string `json:"ticker"`
	Action      string `json:"action"` // "buy" or "sell"
	Side        string `json:"side"`   // "yes" or "no"
	Type        string `json:"type"`   // "limit" or "market"
	Count       int    `json:"count"`
	YesPrice    int    `json:"yes_price,omitempty"`
	NoPrice     int    `json:"no_price,omitempty"`
	TimeInForce string `json:"time_in_force,omitempty"` // "good_till_canceled", "immediate_or_cancel", "fill_or_kill"
}

type Order struct {
	OrderID        string `json:"order_id"`
	Ticker         string `json:"ticker"`
	Status         string `json:"status"`
	Action         string `json:"action"`
	Side           string `json:"side"`
	Type           string `json:"type"`
	YesPrice       int    `json:"yes_price"`
	NoPrice        int    `json:"no_price"`
	RemainingCount int    `json:"remaining_count"`
	FilledCount    int    `json:"place_count"`
}

// --- API Methods ---

func (c *Client) GetMarket(ctx context.Context, ticker string) (*Market, error) {
	var response struct {
		Market Market `json:"market"`
	}
	if err := c.get(ctx, "/markets/"+ticker, nil, &response); err != nil {
		return nil, err
	}
	return &response.Market, nil
}

func (c *Client) GetMarkets(ctx context.Context, seriesTicker string, status string) ([]Market, error) {
	params := url.Values{}
	if seriesTicker != "" {
		params.Set("series_ticker", seriesTicker)
	}
	if status != "" {
		params.Set("status", status)
	}
	params.Set("limit", "200")

	var result struct {
		Markets []Market `json:"markets"`
		Cursor  string   `json:"cursor"`
	}
	if err := c.get(ctx, "/markets", params, &result); err != nil {
		return nil, err
	}
	return result.Markets, nil
}

func (c *Client) GetOrderbook(ctx context.Context, ticker string, depth int) (*Orderbook, error) {
	params := url.Values{}
	if depth > 0 {
		params.Set("depth", fmt.Sprintf("%d", depth))
	}

	var result struct {
		Orderbook Orderbook `json:"orderbook"`
	}
	if err := c.get(ctx, "/markets/"+ticker+"/orderbook", params, &result); err != nil {
		return nil, err
	}
	return &result.Orderbook, nil
}

func (c *Client) GetBalance(ctx context.Context) (*Balance, error) {
	var result Balance
	if err := c.get(ctx, "/portfolio/balance", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetPositions(ctx context.Context, eventTicker string) ([]Position, error) {
	params := url.Values{}
	if eventTicker != "" {
		params.Set("event_ticker", eventTicker)
	}
	params.Set("limit", "200")

	var result struct {
		Positions []Position `json:"market_positions"`
	}
	if err := c.get(ctx, "/portfolio/positions", params, &result); err != nil {
		return nil, err
	}
	return result.Positions, nil
}

func (c *Client) CreateOrder(ctx context.Context, req OrderRequest) (*Order, error) {
	var result struct {
		Order Order `json:"order"`
	}
	if err := c.post(ctx, "/portfolio/orders", req, &result); err != nil {
		return nil, err
	}
	return &result.Order, nil
}

func (c *Client) CancelOrder(ctx context.Context, orderID string) error {
	return c.delete(ctx, "/portfolio/orders/"+orderID)
}

type Fill struct {
	FillID      string `json:"fill_id"`
	OrderID     string `json:"order_id"`
	Ticker      string `json:"ticker"`
	Side        string `json:"side"`
	Action      string `json:"action"`
	Count       int    `json:"count"`
	YesPrice    int    `json:"yes_price"`
	NoPrice     int    `json:"no_price"`
	IsTaker     bool   `json:"is_taker"`
	CreatedTime string `json:"created_time"`
}

func (c *Client) GetFills(ctx context.Context, params url.Values) ([]Fill, string, error) {
	var result struct {
		Fills  []Fill `json:"fills"`
		Cursor string `json:"cursor"`
	}
	if err := c.get(ctx, "/portfolio/fills", params, &result); err != nil {
		return nil, "", err
	}
	return result.Fills, result.Cursor, nil
}

// --- HTTP helpers ---

func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	reqURL := c.baseURL + path
	if params != nil && len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}

	headers, err := AuthHeaders(c.cfg, c.privKey, "GET", c.signPath(path))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req, out)
}

func (c *Client) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, strings.NewReader(string(data)))
	if err != nil {
		return err
	}

	headers, err := AuthHeaders(c.cfg, c.privKey, "POST", c.signPath(path))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return err
	}

	// Fix: use c.signPath(path) for correct DELETE signing
	headers, err := AuthHeaders(c.cfg, c.privKey, "DELETE", c.signPath(path))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.doRequest(req, nil)
}

func (c *Client) doRequest(req *http.Request, out interface{}) error {
	slog.Debug("kalshi request", "method", req.Method, "url", req.URL.String())

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("kalshi request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		slog.Error("kalshi API error", "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("kalshi API error %d: %s", resp.StatusCode, string(body))
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decoding response: %w (body: %s)", err, string(body))
		}
	}

	return nil
}
