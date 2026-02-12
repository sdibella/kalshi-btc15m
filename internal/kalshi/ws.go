package kalshi

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sdibella/kalshi-btc15m/internal/config"
)

// WSClient manages a WebSocket connection to Kalshi for real-time orderbook data.
type WSClient struct {
	cfg     *config.Config
	privKey *rsa.PrivateKey
	conn    *websocket.Conn
	mu      sync.RWMutex

	// orderbooks maps ticker -> OrderbookState
	orderbooks map[string]*OrderbookState
	obMu       sync.RWMutex

	// subscription tracking for auto-resubscribe on reconnect
	subscribedTickers map[string]bool
	subMu             sync.RWMutex
}

// OrderbookState holds the current state of an orderbook for a ticker.
type OrderbookState struct {
	Ticker     string
	Yes        []PriceLevel // sorted best->worst
	No         []PriceLevel
	LastUpdate time.Time // when this orderbook was last updated (snapshot or delta)
}

type PriceLevel struct {
	Price    int
	Quantity int
}

func (ob *OrderbookState) BestYesBid() int {
	if len(ob.Yes) > 0 {
		return ob.Yes[0].Price
	}
	return 0
}

func (ob *OrderbookState) BestYesAsk() int {
	if len(ob.No) > 0 {
		return 100 - ob.No[0].Price
	}
	return 100
}

// AskDepth returns ask-side depth for buying a given side, sorted best
// (lowest ask price) first. Buying YES walks the NO side; buying NO walks
// the YES side. Prices are converted to the buyer's perspective.
func (ob *OrderbookState) AskDepth(side string) []PriceLevel {
	var source []PriceLevel
	if side == "yes" {
		source = ob.No
	} else {
		source = ob.Yes
	}

	levels := make([]PriceLevel, 0, len(source))
	for _, l := range source {
		levels = append(levels, PriceLevel{
			Price:    100 - l.Price,
			Quantity: l.Quantity,
		})
	}
	// source is sorted highest-price-first, so after 100-x conversion
	// the result is already sorted lowest-price-first (best ask first).
	return levels
}

func NewWSClient(cfg *config.Config) (*WSClient, error) {
	key, err := LoadPrivateKey(cfg.KalshiPrivKeyPath)
	if err != nil {
		return nil, err
	}

	return &WSClient{
		cfg:               cfg,
		privKey:           key,
		orderbooks:        make(map[string]*OrderbookState),
		subscribedTickers: make(map[string]bool),
	}, nil
}

// Run connects to the Kalshi WebSocket and processes messages.
func (ws *WSClient) Run(ctx context.Context) error {
	for {
		if err := ws.connect(ctx); err != nil {
			slog.Warn("kalshi ws disconnected", "err", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			slog.Info("kalshi ws reconnecting...")
		}
	}
}

func (ws *WSClient) connect(ctx context.Context) error {
	wsURL := ws.cfg.WSBaseURL()

	// Generate auth headers for the WS handshake
	headers, err := AuthHeaders(ws.cfg, ws.privKey, "GET", "/trade-api/ws/v2")
	if err != nil {
		return fmt.Errorf("generating ws auth: %w", err)
	}

	dialer := websocket.Dialer{}
	httpHeaders := make(map[string][]string)
	for k, v := range headers {
		httpHeaders[k] = []string{v}
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, httpHeaders)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.mu.Unlock()

	defer func() {
		conn.Close()
		ws.mu.Lock()
		ws.conn = nil
		ws.mu.Unlock()
	}()

	slog.Info("kalshi ws connected")

	// Re-subscribe to any previously tracked tickers
	if tickers := ws.subscribedTickerList(); len(tickers) > 0 {
		if err := ws.sendSubscribe(conn, tickers); err != nil {
			slog.Warn("kalshi ws resubscribe failed", "err", err, "tickers", len(tickers))
		} else {
			slog.Info("kalshi ws resubscribed", "tickers", len(tickers))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		ws.handleMessage(msg)
	}
}

// Subscribe sends a subscription command for orderbook_delta on the given tickers.
// Tickers are tracked so they are automatically re-subscribed on reconnect.
func (ws *WSClient) Subscribe(tickers []string) error {
	ws.subMu.Lock()
	for _, t := range tickers {
		ws.subscribedTickers[t] = true
	}
	ws.subMu.Unlock()

	ws.mu.RLock()
	conn := ws.conn
	ws.mu.RUnlock()

	if conn == nil {
		// Not connected yet -- tickers are tracked and will be subscribed on connect
		return nil
	}

	return ws.sendSubscribe(conn, tickers)
}

// Unsubscribe removes tickers from tracking (used when markets settle).
func (ws *WSClient) Unsubscribe(tickers []string) {
	ws.subMu.Lock()
	for _, t := range tickers {
		delete(ws.subscribedTickers, t)
	}
	ws.subMu.Unlock()

	ws.obMu.Lock()
	for _, t := range tickers {
		delete(ws.orderbooks, t)
	}
	ws.obMu.Unlock()
}

func (ws *WSClient) sendSubscribe(conn *websocket.Conn, tickers []string) error {
	cmd := wsCommand{
		ID:  1,
		Cmd: "subscribe",
		Params: wsSubscribeParams{
			Channels:      []string{"orderbook_delta"},
			MarketTickers: tickers,
		},
	}
	return conn.WriteJSON(cmd)
}

func (ws *WSClient) subscribedTickerList() []string {
	ws.subMu.RLock()
	defer ws.subMu.RUnlock()
	tickers := make([]string, 0, len(ws.subscribedTickers))
	for t := range ws.subscribedTickers {
		tickers = append(tickers, t)
	}
	return tickers
}

// GetOrderbook returns the current orderbook state for a ticker.
func (ws *WSClient) GetOrderbook(ticker string) *OrderbookState {
	ws.obMu.RLock()
	defer ws.obMu.RUnlock()
	return ws.orderbooks[ticker]
}

type wsCommand struct {
	ID     int              `json:"id"`
	Cmd    string           `json:"cmd"`
	Params wsSubscribeParams `json:"params"`
}

type wsSubscribeParams struct {
	Channels      []string `json:"channels"`
	MarketTickers []string `json:"market_tickers"`
}

type wsMessage struct {
	Type string          `json:"type"`
	Msg  json.RawMessage `json:"msg"`
}

type wsOrderbookSnapshot struct {
	Ticker string  `json:"market_ticker"`
	Yes    [][]int `json:"yes"` // [[price, qty], ...]
	No     [][]int `json:"no"`
}

type wsOrderbookDelta struct {
	Ticker string `json:"market_ticker"`
	Price  int    `json:"price"`
	Delta  int    `json:"delta"`
	Side   string `json:"side"` // "yes" or "no"
}

func (ws *WSClient) handleMessage(data []byte) {
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "orderbook_snapshot":
		var snap wsOrderbookSnapshot
		if err := json.Unmarshal(msg.Msg, &snap); err != nil {
			slog.Warn("bad orderbook snapshot", "err", err)
			return
		}
		ws.applySnapshot(snap)

	case "orderbook_delta":
		var delta wsOrderbookDelta
		if err := json.Unmarshal(msg.Msg, &delta); err != nil {
			slog.Warn("bad orderbook delta", "err", err)
			return
		}
		ws.applyDelta(delta)

	default:
		slog.Info("kalshi ws unhandled message", "type", msg.Type, "msg", string(msg.Msg))
	}
}

func (ws *WSClient) applySnapshot(snap wsOrderbookSnapshot) {
	ob := &OrderbookState{Ticker: snap.Ticker}

	for _, level := range snap.Yes {
		if len(level) >= 2 {
			ob.Yes = append(ob.Yes, PriceLevel{Price: level[0], Quantity: level[1]})
		}
	}
	for _, level := range snap.No {
		if len(level) >= 2 {
			ob.No = append(ob.No, PriceLevel{Price: level[0], Quantity: level[1]})
		}
	}

	ob.LastUpdate = time.Now()

	ws.obMu.Lock()
	ws.orderbooks[snap.Ticker] = ob
	ws.obMu.Unlock()

	slog.Debug("orderbook snapshot", "ticker", snap.Ticker, "yesLevels", len(ob.Yes), "noLevels", len(ob.No))
}

func (ws *WSClient) applyDelta(delta wsOrderbookDelta) {
	ws.obMu.Lock()
	defer ws.obMu.Unlock()

	ob := ws.orderbooks[delta.Ticker]
	if ob == nil {
		return
	}
	ob.LastUpdate = time.Now()

	var levels *[]PriceLevel
	if delta.Side == "yes" {
		levels = &ob.Yes
	} else {
		levels = &ob.No
	}

	// Find existing level at this price
	for i, l := range *levels {
		if l.Price == delta.Price {
			newQty := l.Quantity + delta.Delta
			if newQty <= 0 {
				*levels = append((*levels)[:i], (*levels)[i+1:]...)
			} else {
				(*levels)[i].Quantity = newQty
			}
			return
		}
	}

	// New price level
	if delta.Delta > 0 {
		*levels = append(*levels, PriceLevel{Price: delta.Price, Quantity: delta.Delta})
		// Keep sorted (highest price first for yes, highest for no)
		for i := len(*levels) - 1; i > 0; i-- {
			if (*levels)[i].Price > (*levels)[i-1].Price {
				(*levels)[i], (*levels)[i-1] = (*levels)[i-1], (*levels)[i]
			}
		}
	}
}
