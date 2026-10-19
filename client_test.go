package layerhop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type recorded struct {
	method, path, rawPath, query, body string
	header                             http.Header
}

func newServer(t *testing.T, status int, resp string) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*rec = recorded{r.Method, r.URL.Path, r.URL.EscapedPath(), r.URL.RawQuery, string(b), r.Header.Clone()}
		w.Header().Set("content-type", "application/json")
		w.Header().Set("x-request-id", "req-hdr")
		w.WriteHeader(status)
		io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return New(Options{BaseURL: srv.URL + "/", APIKey: "k1"}), rec
}

func ctx() context.Context { return context.Background() }

func TestDefaults(t *testing.T) {
	c := New(Options{})
	if c.baseURL != DefaultBaseURL || c.opts.Channel != "api" || !strings.HasPrefix(c.opts.UserAgent, "layerhop-go/") {
		t.Fatalf("bad defaults: %+v %s", c.opts, c.baseURL)
	}
}

func TestHealthChainsTokens(t *testing.T) {
	c, rec := newServer(t, 200, `{"ok":true,"providers":["relay","mock"]}`)
	h, err := c.Health(ctx())
	if err != nil || !h.OK || len(h.Providers) != 2 || rec.path != "/v1/health" {
		t.Fatalf("health: %+v %v %s", h, err, rec.path)
	}
	if rec.header.Get("x-api-key") != "k1" || rec.header.Get("user-agent") != "layerhop-go/"+Version {
		t.Fatalf("headers: %v", rec.header)
	}

	c, rec = newServer(t, 200, `{"chains":[{"id":7565164,"slug":"solana","name":"Solana","vm":"svm","nativeSymbol":"SOL","nativeDecimals":9,"explorerTx":"https://solscan.io/tx/"}]}`)
	ch, err := c.Chains(ctx())
	if err != nil || len(ch) != 1 || ch[0].ID != SolanaChainID || rec.path != "/v1/chains" {
		t.Fatalf("chains: %+v %v", ch, err)
	}

	c, rec = newServer(t, 200, `{"tokens":[{"chainId":8453,"address":"0x83","symbol":"USDC","decimals":6}]}`)
	tk, err := c.Tokens(ctx(), "base")
	if err != nil || tk[0].Symbol != "USDC" || rec.query != "chain=base" {
		t.Fatalf("tokens: %+v %v %s", tk, err, rec.query)
	}
}

const quoteJSON = `{"best":{"quoteId":"q_1","provider":"relay","dstAmount":"612345678","dstAmountMin":"609283949","estimatedSeconds":8,"fees":{"ourFeeBps":20,"providerFeeUsd":0.12,"integratorFeeBps":10,"integrator":"myapp"},"expiresAt":1760000030},"quotes":[{"quoteId":"q_1","provider":"relay","dstAmount":"612345678","dstAmountMin":"609283949","estimatedSeconds":8,"fees":{"ourFeeBps":20},"expiresAt":1760000030}],"errors":[{"provider":"lifi","code":"PROVIDER_TIMEOUT","message":"Timed out"}]}`

func TestQuote(t *testing.T) {
	c, rec := newServer(t, 200, quoteJSON)
	fee := 25
	c.opts.Integrator = "myapp"
	c.opts.IntegratorFeeBps = &fee
	q, err := c.Quote(ctx(), QuoteParams{SrcChain: "base", SrcToken: "USDC", Amount: "100000000", DstChain: "solana", DstToken: "SOL", Sender: "0xabc", Recipient: "9Wz"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Best.QuoteID != "q_1" || q.Best.DstAmount != "612345678" || *q.Best.Fees.IntegratorFeeBps != 10 || len(q.Errors) != 1 {
		t.Fatalf("decoded: %+v", q)
	}
	if rec.method != "GET" || rec.path != "/v1/quote" {
		t.Fatalf("route: %s %s", rec.method, rec.path)
	}
	want := "amount=100000000&channel=api&dstChain=solana&dstToken=SOL&integrator=myapp&integratorFeeBps=25&recipient=9Wz&sender=0xabc&slippageBps=50&srcChain=base&srcToken=USDC"
	if rec.query != want {
		t.Fatalf("query\n got %s\nwant %s", rec.query, want)
	}
}

func TestQuoteGasOnArrival(t *testing.T) {
	resp := strings.Replace(quoteJSON, `"expiresAt":1760000030},"quotes"`,
		`"expiresAt":1760000030,"gasOnArrival":{"amount":"512000000000000","token":"0x0000000000000000000000000000000000000000","symbol":"ETH","decimals":18,"usd":2}},"quotes"`, 1)
	c, rec := newServer(t, 200, resp)
	p := QuoteParams{SrcChain: "solana", SrcToken: "USDC", Amount: "100000000", DstChain: "base", DstToken: "USDC", Sender: "9Wz", Recipient: "0xabc", GasOnArrival: true}
	q, err := c.Quote(ctx(), p)
	if err != nil {
		t.Fatal(err)
	}
	g := q.Best.GasOnArrival
	if g == nil || g.Amount != "512000000000000" || g.Symbol != "ETH" || g.Decimals != 18 || g.Usd == nil || *g.Usd != 2 || q.Quotes[0].GasOnArrival != nil {
		t.Fatalf("gas on arrival: %+v", g)
	}
	if !strings.Contains(rec.query, "&gasOnArrival=true&") || strings.Contains(rec.query, "gasOnArrivalUsd") {
		t.Fatalf("query: %s", rec.query)
	}

	c, rec = newServer(t, 200, quoteJSON)
	p.GasOnArrivalUsd = 5.5
	if _, err := c.Quote(ctx(), p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.query, "&gasOnArrivalUsd=5.5&") || strings.Contains(rec.query, "gasOnArrival=") {
		t.Fatalf("usd query: %s", rec.query)
	}

	c, rec = newServer(t, 200, quoteJSON)
	p.GasOnArrival, p.GasOnArrivalUsd = false, 0
	if _, err := c.Quote(ctx(), p); err != nil || strings.Contains(rec.query, "gasOnArrival") {
		t.Fatalf("off: %s %v", rec.query, err)
	}
}

func TestBuildTxEvmAndSvm(t *testing.T) {
	c, rec := newServer(t, 200, `{"quoteId":"q_1","provider":"relay","tx":{"vm":"evm","chainId":8453,"to":"0xto","data":"0xdead","value":"0","approval":{"token":"0xt","spender":"0xs","amount":"100"}}}`)
	tx, err := c.BuildTx(ctx(), "q_1")
	if err != nil || !tx.IsEVM() || tx.IsSVM() || tx.ChainID != 8453 || tx.Approval == nil || tx.Approval.Amount != "100" {
		t.Fatalf("evm tx: %+v %v", tx, err)
	}
	if rec.method != "POST" || rec.path != "/v1/orders" || rec.body != `{"quoteId":"q_1"}` || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}

	c, _ = newServer(t, 200, `{"quoteId":"q_2","provider":"jupiter","tx":{"vm":"svm","serializedTx":"AQID","submitViaApi":true}}`)
	tx, err = c.BuildTx(ctx(), "q_2")
	if err != nil || !tx.IsSVM() || tx.SerializedTx != "AQID" || !tx.SubmitViaAPI {
		t.Fatalf("svm tx: %+v %v", tx, err)
	}
}

func TestSubmit(t *testing.T) {
	c, rec := newServer(t, 200, `{"provider":"jupiter","txHash":"5VER"}`)
	h, err := c.Submit(ctx(), "q_1", "c2lnbmVk")
	if err != nil || h != "5VER" || rec.path != "/v1/orders/submit" {
		t.Fatalf("submit: %s %v", h, err)
	}
	var b map[string]string
	json.Unmarshal([]byte(rec.body), &b)
	if b["quoteId"] != "q_1" || b["signedTransaction"] != "c2lnbmVk" {
		t.Fatalf("body: %s", rec.body)
	}
}

func TestReportSubmitted(t *testing.T) {
	c, rec := newServer(t, 200, `{"quoteId":"q_1","status":"submitted"}`)
	r, err := c.ReportSubmitted(ctx(), "q_1", "0xhash")
	if err != nil || r.Status != "submitted" || r.QuoteID != "q_1" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "POST" || rec.path != "/v1/orders/q_1/submitted" || rec.body != `{"txHash":"0xhash"}` {
		t.Fatalf("req: %+v", rec)
	}
}

func TestSimulate(t *testing.T) {
	c, rec := newServer(t, 200, `{"ok":false,"vm":"evm","gasEstimate":"21000","needsApproval":true,"error":{"code":"SIMULATION_FAILED","message":"reverted","logs":["l1"]},"tx":{"vm":"evm","chainId":1,"to":"0x","data":"0x","value":"0"}}`)
	s, err := c.Simulate(ctx(), "q_1")
	if err != nil || s.OK || s.GasEstimate != "21000" || !s.NeedsApproval || s.Error.Logs[0] != "l1" || !s.Tx.IsEVM() {
		t.Fatalf("%+v %v", s, err)
	}
	if rec.path != "/v1/simulate" || rec.body != `{"quoteId":"q_1"}` {
		t.Fatalf("req: %+v", rec)
	}

	// Numeric gas / compute units are accepted too.
	c, _ = newServer(t, 200, `{"ok":true,"vm":"svm","computeUnits":120000}`)
	s, err = c.Simulate(ctx(), "q_2")
	if err != nil || !s.OK || s.ComputeUnits != "120000" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestStatusEscapesID(t *testing.T) {
	c, rec := newServer(t, 200, `{"provider":"debridge","id":"a/b c","state":"filled","srcTxHash":"0x1","dstTxHash":"sig"}`)
	s, err := c.Status(ctx(), "debridge", "a/b c")
	if err != nil || s.State != StateFilled || s.DstTxHash != "sig" {
		t.Fatalf("%+v %v", s, err)
	}
	if rec.rawPath != "/v1/orders/debridge/a%2Fb%20c" {
		t.Fatalf("path: %s", rec.rawPath)
	}
}

func TestListOrders(t *testing.T) {
	c, rec := newServer(t, 200, `{"orders":[{"quoteId":"q_1","provider":"relay","status":"filled","srcChainId":8453,"amount":"100","sender":"0xabc"}],"nextCursor":"c2"}`)
	r, err := c.ListOrders(ctx(), ListOrdersParams{Address: "0xabc", Status: "filled", Cursor: "c1", Limit: 10})
	if err != nil || len(r.Orders) != 1 || r.Orders[0].SrcChainID != 8453 || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/orders" || rec.query != "address=0xabc&cursor=c1&limit=10&status=filled" {
		t.Fatalf("req: %+v", rec)
	}
	_, _ = c.ListOrders(ctx(), ListOrdersParams{})
	if rec.query != "" {
		t.Fatalf("empty params should send no query: %s", rec.query)
	}
}

func TestAPIError(t *testing.T) {
	c, _ := newServer(t, 410, `{"error":{"code":"QUOTE_EXPIRED","message":"Quote expired, request a new one","requestId":"req-1","docsUrl":"x"}}`)
	_, err := c.BuildTx(ctx(), "q_old")
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("not APIError: %v", err)
	}
	if ae.Code != "QUOTE_EXPIRED" || ae.Status != 410 || ae.RequestID != "req-1" || !strings.Contains(ae.Error(), "QUOTE_EXPIRED") || ae.DocsURL() != "https://docs.layerhop.com/errors/QUOTE_EXPIRED" {
		t.Fatalf("%+v", ae)
	}

	c, _ = newServer(t, 502, `not json`)
	_, err = c.Chains(ctx())
	if !errors.As(err, &ae) || ae.Code != "INTERNAL" || ae.Status != 502 || ae.RequestID != "req-hdr" || ae.Message != "Bad Gateway" {
		t.Fatalf("fallback: %+v", ae)
	}
}

func TestContextCancel(t *testing.T) {
	c, _ := newServer(t, 200, `{}`)
	cx, cancel := context.WithCancel(ctx())
	cancel()
	if _, err := c.Health(cx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
}

func statusSequence(t *testing.T, states ...string) (*Client, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(states) {
			i = len(states) - 1
		}
		io.WriteString(w, `{"provider":"relay","id":"0x1","state":"`+states[i]+`"}`)
	}))
	t.Cleanup(srv.Close)
	return New(Options{BaseURL: srv.URL}), &n
}

func TestWatchYieldsChangesUntilFinal(t *testing.T) {
	c, n := statusSequence(t, "unknown", "pending", "pending", "filled", "pending")
	var got []string
	for st, err := range c.Watch(ctx(), "relay", "0x1", WatchOptions{Interval: time.Millisecond}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, st.State)
	}
	if strings.Join(got, ",") != "unknown,pending,filled" || n.Load() != 4 {
		t.Fatalf("got %v after %d polls", got, n.Load())
	}
}

func TestWatchBreakAndTimeout(t *testing.T) {
	c, n := statusSequence(t, "pending")
	for range c.Watch(ctx(), "relay", "0x1", WatchOptions{Interval: time.Millisecond}) {
		break
	}
	if n.Load() != 1 {
		t.Fatalf("break should stop polling, polls=%d", n.Load())
	}

	var last error
	for _, err := range c.Watch(ctx(), "relay", "0x1", WatchOptions{Interval: 5 * time.Millisecond, Timeout: 20 * time.Millisecond}) {
		last = err
	}
	if !errors.Is(last, ErrWatchTimeout) {
		t.Fatalf("want timeout, got %v", last)
	}

	cx, cancel := context.WithTimeout(ctx(), 15*time.Millisecond)
	defer cancel()
	for _, err := range c.Watch(cx, "relay", "0x1", WatchOptions{Interval: 5 * time.Millisecond}) {
		last = err
	}
	if !errors.Is(last, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", last)
	}
}

func TestWatchStopsOnError(t *testing.T) {
	c, _ := newServer(t, 404, `{"error":{"code":"ORDER_NOT_FOUND","message":"nope"}}`)
	count := 0
	var ae *APIError
	for _, err := range c.Watch(ctx(), "relay", "0x1", WatchOptions{Interval: time.Millisecond}) {
		count++
		if !errors.As(err, &ae) || ae.Code != "ORDER_NOT_FOUND" {
			t.Fatalf("%v", err)
		}
	}
	if count != 1 {
		t.Fatalf("count %d", count)
	}
}

func TestIsFinal(t *testing.T) {
	for s, want := range map[string]bool{"filled": true, "refunded": true, "failed": true, "pending": false, "unknown": false} {
		if IsFinal(s) != want {
			t.Fatalf("%s", s)
		}
	}
}
