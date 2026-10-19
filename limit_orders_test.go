package layerhop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const limitOrderJSON = `{"id":"lo_01m5h2k2c4x8q6r0v3b5n7p9d","object":"limit_order","livemode":false,"status":"built","statusDetail":null,"provider":"sandbox",` +
	`"chainId":7565164,"wallet":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","srcToken":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",` +
	`"dstToken":"So11111111111111111111111111111111111111112","amount":"100000000","dstAmount":"600000000","expiresAt":null,` +
	`"fees":{"ourFeeBps":20,"integratorFeeBps":0,"integrator":null},"providerOrderId":"7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",` +
	`"filledAmount":"0","filledDstAmount":"0","fillCount":0,"openTxHash":null,"cancelTxHash":null,"channel":"api","metadata":{"userId":"42"},` +
	`"createdAt":"2026-10-19T10:00:00.000Z","updatedAt":"2026-10-19T10:00:00.000Z"}`

const sandboxSvmTxJSON = `{"vm":"svm","serializedTx":"AQAB","submitViaApi":true,"sandbox":true}`

const fillOrderJSON = `{"quoteId":"lof_5f0c1e2d","provider":"sandbox","status":"filled","statusDetail":null,"channel":"api","ourFeeBps":20,"partnerFeeBps":0,` +
	`"sender":"9Wz","recipient":"9Wz","srcChainId":7565164,"srcToken":"EPj","dstChainId":7565164,"dstToken":"So1","amount":"50000000","dstAmount":"300000000",` +
	`"srcTxHash":"5sig","dstTxHash":"5sig","livemode":false,"checkoutSessionId":null,"refundId":null,"depositAddressId":null,` +
	`"limitOrderId":"lo_01m5h2k2c4x8q6r0v3b5n7p9d","createdAt":"t","updatedAt":"t"}`

func TestCreateLimitOrder(t *testing.T) {
	c, rec := newServer(t, 201, withExtra(limitOrderJSON, `"tx":`+sandboxSvmTxJSON))
	o, err := c.CreateLimitOrder(ctx(), CreateLimitOrderParams{
		Chain: "solana", Wallet: "9Wz", SrcToken: "USDC", DstToken: "SOL", Amount: "100000000", DstAmount: "600000000",
		ExpiresAt: 1760003600, Metadata: map[string]string{"userId": "42"}, Integrator: "acme", IntegratorFeeBps: ptr(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/limit-orders" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"chain":"solana","wallet":"9Wz","srcToken":"USDC","dstToken":"SOL","amount":"100000000","dstAmount":"600000000","expiresAt":1760003600,"channel":"api","metadata":{"userId":"42"},"integrator":"acme","integratorFeeBps":10}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if o.ID != "lo_01m5h2k2c4x8q6r0v3b5n7p9d" || o.Object != "limit_order" || o.Livemode || o.Status != LimitOrderBuilt || o.Provider != "sandbox" ||
		o.ChainID != SolanaChainID || o.ExpiresAt != "" || o.Fees.OurFeeBps != 20 || o.Fees.Integrator != "" || o.FilledAmount != "0" || o.FillCount != 0 ||
		o.OpenTxHash != "" || o.Channel != "api" || o.Metadata["userId"] != "42" || o.Tx == nil || !o.Tx.IsSVM() || !o.Tx.SubmitViaAPI || !o.Tx.Sandbox || o.Fills != nil {
		t.Fatalf("limit order: %+v", o)
	}

	// Minimal body; channel, Integrator and IntegratorFeeBps come from the client.
	c, rec = newServer(t, 201, withExtra(limitOrderJSON, `"tx":`+sandboxSvmTxJSON))
	c.opts.Channel, c.opts.Integrator, c.opts.IntegratorFeeBps = "widget", "acme", ptr(5)
	if _, err := c.CreateLimitOrder(ctx(), CreateLimitOrderParams{Wallet: "9Wz", SrcToken: "USDC", DstToken: "SOL", Amount: "1", DstAmount: "2"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"wallet":"9Wz","srcToken":"USDC","dstToken":"SOL","amount":"1","dstAmount":"2","channel":"widget","integrator":"acme","integratorFeeBps":5}` {
		t.Fatalf("defaults body: %s", rec.body)
	}

	// Live mode disabled by the operator.
	c, _ = newServer(t, 403, `{"error":{"code":"FORBIDDEN","message":"Limit orders are not enabled for live mode yet.","requestId":"r1"}}`)
	_, err = c.CreateLimitOrder(ctx(), CreateLimitOrderParams{Wallet: "9Wz", SrcToken: "USDC", DstToken: "SOL", Amount: "1", DstAmount: "2"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" || ae.Status != 403 {
		t.Fatalf("%v", err)
	}
}

func TestSubmitGetListLimitOrders(t *testing.T) {
	open := strings.Replace(limitOrderJSON, `"status":"built"`, `"status":"open"`, 1)
	c, rec := newServer(t, 200, `{"limitOrder":`+open+`,"txHash":"5sig"}`)
	r, err := c.SubmitLimitOrder(ctx(), "lo_1", "c2lnbmVk")
	if err != nil || r.TxHash != "5sig" || r.LimitOrder.Status != LimitOrderOpen {
		t.Fatalf("submit: %+v %v", r, err)
	}
	if rec.method != "POST" || rec.path != "/v1/limit-orders/lo_1/submit" || rec.body != `{"signedTransaction":"c2lnbmVk"}` {
		t.Fatalf("req: %+v", rec)
	}

	c, rec = newServer(t, 200, withExtra(strings.Replace(open, `"fillCount":0`, `"fillCount":1`, 1), `"fills":[`+fillOrderJSON+`]`))
	o, err := c.GetLimitOrder(ctx(), "lo_1")
	if err != nil || rec.method != "GET" || rec.path != "/v1/limit-orders/lo_1" {
		t.Fatalf("get: %+v %v %+v", o, err, rec)
	}
	if o.FillCount != 1 || len(o.Fills) != 1 || o.Fills[0].LimitOrderID != "lo_01m5h2k2c4x8q6r0v3b5n7p9d" || o.Fills[0].QuoteID != "lof_5f0c1e2d" || o.Tx != nil {
		t.Fatalf("fills: %+v", o)
	}
	c, rec = newServer(t, 200, withExtra(limitOrderJSON, `"fills":[]`))
	if _, err := c.GetLimitOrder(ctx(), "a/b"); err != nil || rec.rawPath != "/v1/limit-orders/a%2Fb" {
		t.Fatalf("encoded: %s %v", rec.rawPath, err)
	}

	c, rec = newServer(t, 200, `{"limitOrders":[`+limitOrderJSON+`],"nextCursor":"c2"}`)
	l, err := c.ListLimitOrders(ctx(), ListLimitOrdersParams{Wallet: "9Wz", Status: LimitOrderOpen, Cursor: "c1", Limit: 10})
	if err != nil || len(l.LimitOrders) != 1 || l.NextCursor != "c2" {
		t.Fatalf("%+v %v", l, err)
	}
	if rec.method != "GET" || rec.path != "/v1/limit-orders" || rec.query != "cursor=c1&limit=10&status=open&wallet=9Wz" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"limitOrders":[],"nextCursor":null}`)
	l, err = c.ListLimitOrders(ctx(), ListLimitOrdersParams{})
	if err != nil || rec.query != "" || l.NextCursor != "" || len(l.LimitOrders) != 0 {
		t.Fatalf("empty: %s %+v %v", rec.query, l, err)
	}
}

func TestCancelSimulateLimitOrder(t *testing.T) {
	open := strings.Replace(limitOrderJSON, `"status":"built"`, `"status":"open"`, 1)
	c, rec := newServer(t, 200, `{"limitOrder":`+open+`,"tx":{"vm":"svm","serializedTx":"AQAC","submitViaApi":true}}`)
	r, err := c.CancelLimitOrder(ctx(), "lo_1")
	if err != nil || r.Tx == nil || r.Tx.SerializedTx != "AQAC" || r.LimitOrder.Status != LimitOrderOpen || rec.method != "POST" || rec.path != "/v1/limit-orders/lo_1/cancel" || rec.body != `{}` {
		t.Fatalf("cancel open: %+v %v %+v", r, err, rec)
	}
	c, _ = newServer(t, 200, `{"limitOrder":`+strings.Replace(limitOrderJSON, `"status":"built"`, `"status":"cancelled"`, 1)+`,"tx":null}`)
	r, err = c.CancelLimitOrder(ctx(), "lo_1")
	if err != nil || r.Tx != nil || r.LimitOrder.Status != LimitOrderCancelled {
		t.Fatalf("cancel built: %+v %v", r, err)
	}

	c, rec = newServer(t, 202, open)
	o, err := c.SimulateLimitOrder(ctx(), "lo_1", SimulatePartialFill)
	if err != nil || o.Status != LimitOrderOpen || rec.method != "POST" || rec.path != "/v1/limit-orders/lo_1/simulate" || rec.body != `{"outcome":"partial_fill"}` {
		t.Fatalf("simulate: %+v %v %+v", o, err, rec)
	}
	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"This order is filled","requestId":"r2"}}`)
	_, err = c.CancelLimitOrder(ctx(), "lo_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}

func TestLimitOrderWebhookEvent(t *testing.T) {
	body := `{"id":"evt_1","type":"limit_order.filled","livemode":false,"createdAt":"t","data":{"limitOrder":` + limitOrderJSON + `,"fill":` + fillOrderJSON + `}}`
	var e WebhookEvent
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != EventLimitOrderFilled || e.Data.LimitOrder == nil || e.Data.LimitOrder.ID != "lo_01m5h2k2c4x8q6r0v3b5n7p9d" || e.Data.Fill == nil || e.Data.Fill.LimitOrderID == "" {
		t.Fatalf("%+v", e)
	}
	var o Order
	if err := json.Unmarshal([]byte(fillOrderJSON), &o); err != nil || o.LimitOrderID != "lo_01m5h2k2c4x8q6r0v3b5n7p9d" {
		t.Fatalf("%+v %v", o, err)
	}
}
