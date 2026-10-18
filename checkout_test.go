package layerhop

import (
	"encoding/json"
	"errors"
	"testing"
)

const sessionJSON = `{"id":"cs_01k7t9m2c4x8q6r0v3b5n7p9d","object":"checkout.session","livemode":true,"status":"processing","statusDetail":null,"amount":"49000000",` +
	`"settlement":{"chainId":8453,"token":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","symbol":"USDC","decimals":6},` +
	`"recipient":"0x2222222222222222222222222222222222222222","merchantName":"Acme","description":"Pro plan, 1 month",` +
	`"url":"https://layerhop.com/pay/cs_01k7t9m2c4x8q6r0v3b5n7p9d","successUrl":"https://shop.example.com/thanks","cancelUrl":null,"expiresAt":"2026-10-18T11:00:00.000Z",` +
	`"payment":{"quoteId":"q_9","payer":"0x1111111111111111111111111111111111111111","srcChainId":42161,"srcToken":"0x0000000000000000000000000000000000000000","srcAmount":"15623001000000000","srcTxHash":"0xabc","dstTxHash":null,"paidAt":null},` +
	`"reference":"order_1042","metadata":{"plan":"pro"},"createdAt":"2026-10-18T10:00:00.000Z","updatedAt":"2026-10-18T10:02:00.000Z"}`

const checkoutQuoteJSON = `{"sessionId":"cs_01k7t9m2c4x8q6r0v3b5n7p9d","livemode":false,"amountDue":"49000000",` +
	`"best":{"quoteId":"q_1","provider":"sandbox","srcAmount":"15623001000000000","dstAmount":"49100000","dstAmountMin":"49000000","estimatedSeconds":8,"fees":{"ourFeeBps":20},"expiresAt":1760000030},` +
	`"quotes":[{"quoteId":"q_1","provider":"sandbox","srcAmount":"15623001000000000","dstAmount":"49100000","dstAmountMin":"49000000","estimatedSeconds":8,"fees":{"ourFeeBps":20},"expiresAt":1760000030}],` +
	`"errors":[{"provider":"lifi","code":"PROVIDER_TIMEOUT","message":"Timed out"}]}`

func TestCreateCheckoutSession(t *testing.T) {
	c, rec := newServer(t, 201, sessionJSON)
	s, err := c.CreateCheckoutSession(ctx(), CreateCheckoutParams{
		Amount: "49000000", SettleChain: "base", SettleToken: "USDC", Recipient: "0x2222",
		Description: "Pro plan, 1 month", Reference: "order_1042", Metadata: map[string]string{"plan": "pro"},
		SuccessURL: "https://shop.example.com/thanks", ExpiresInMinutes: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/checkout/sessions" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"amount":"49000000","settleChain":"base","settleToken":"USDC","recipient":"0x2222","description":"Pro plan, 1 month","reference":"order_1042","metadata":{"plan":"pro"},"successUrl":"https://shop.example.com/thanks","expiresInMinutes":30}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if s.ID != "cs_01k7t9m2c4x8q6r0v3b5n7p9d" || s.Object != "checkout.session" || !s.Livemode || s.Status != CheckoutProcessing {
		t.Fatalf("session: %+v", s)
	}
	if s.Settlement.ChainID != 8453 || s.Settlement.Decimals != 6 || s.URL == "" || s.CancelURL != "" || s.StatusDetail != "" || s.Metadata["plan"] != "pro" {
		t.Fatalf("session fields: %+v", s)
	}
	if s.Payment == nil || s.Payment.SrcChainID != 42161 || s.Payment.SrcAmount != "15623001000000000" || s.Payment.DstTxHash != "" || s.Payment.PaidAt != "" {
		t.Fatalf("payment: %+v", s.Payment)
	}

	// Only the required fields are sent when the optional ones are empty.
	c, rec = newServer(t, 201, sessionJSON)
	if _, err := c.CreateCheckoutSession(ctx(), CreateCheckoutParams{Amount: "1", SettleChain: "8453", SettleToken: "USDC", Recipient: "0x2"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"amount":"1","settleChain":"8453","settleToken":"USDC","recipient":"0x2"}` {
		t.Fatalf("minimal body: %s", rec.body)
	}
}

func TestGetAndExpireCheckoutSession(t *testing.T) {
	c, rec := newServer(t, 200, `{"id":"cs_1","object":"checkout.session","livemode":false,"status":"open","amount":"1","settlement":{"chainId":8453,"token":"0x83","symbol":"USDC","decimals":6},"recipient":"0x2","url":"u","expiresAt":"e","payment":null,"createdAt":"c","updatedAt":"u"}`)
	s, err := c.GetCheckoutSession(ctx(), "cs_1")
	if err != nil || s.Payment != nil || s.Livemode || s.Status != CheckoutOpen || s.Reference != "" {
		t.Fatalf("%+v %v", s, err)
	}
	if rec.method != "GET" || rec.path != "/v1/checkout/sessions/cs_1" || rec.body != "" {
		t.Fatalf("req: %+v", rec)
	}

	c, rec = newServer(t, 200, `{"id":"a/b","status":"expired","payment":null}`)
	s, err = c.ExpireCheckoutSession(ctx(), "a/b")
	if err != nil || s.Status != CheckoutExpired {
		t.Fatalf("%+v %v", s, err)
	}
	if rec.method != "POST" || rec.rawPath != "/v1/checkout/sessions/a%2Fb/expire" || rec.body != `{}` {
		t.Fatalf("req: %+v", rec)
	}

	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"Session is paid","requestId":"r1"}}`)
	_, err = c.ExpireCheckoutSession(ctx(), "cs_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}

func TestListCheckoutSessions(t *testing.T) {
	c, rec := newServer(t, 200, `{"sessions":[`+sessionJSON+`],"nextCursor":"c2"}`)
	r, err := c.ListCheckoutSessions(ctx(), ListCheckoutSessionsParams{Status: CheckoutPaid, Cursor: "c1", Limit: 10})
	if err != nil || len(r.Sessions) != 1 || r.Sessions[0].Payment.QuoteID != "q_9" || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/checkout/sessions" || rec.query != "cursor=c1&limit=10&status=paid" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"sessions":[],"nextCursor":null}`)
	r, err = c.ListCheckoutSessions(ctx(), ListCheckoutSessionsParams{})
	if err != nil || rec.query != "" || r.NextCursor != "" {
		t.Fatalf("empty params: %s %+v %v", rec.query, r, err)
	}
}

func TestQuoteCheckoutSession(t *testing.T) {
	c, rec := newServer(t, 200, checkoutQuoteJSON)
	r, err := c.QuoteCheckoutSession(ctx(), "cs_01k7t9m2c4x8q6r0v3b5n7p9d", CheckoutQuoteParams{SrcChain: "arbitrum", SrcToken: "ETH", Sender: "0x11"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/checkout/sessions/cs_01k7t9m2c4x8q6r0v3b5n7p9d/quote" || rec.body != `{"srcChain":"arbitrum","srcToken":"ETH","sender":"0x11"}` {
		t.Fatalf("req: %+v", rec)
	}
	if r.SessionID != "cs_01k7t9m2c4x8q6r0v3b5n7p9d" || r.Livemode || r.AmountDue != "49000000" || len(r.Quotes) != 1 || r.Errors[0].Code != "PROVIDER_TIMEOUT" {
		t.Fatalf("decoded: %+v", r)
	}
	if r.Best.QuoteID != "q_1" || r.Best.Provider != "sandbox" || r.Best.SrcAmount != "15623001000000000" || r.Best.DstAmountMin != "49000000" || r.Best.Fees.OurFeeBps != 20 {
		t.Fatalf("best: %+v", r.Best)
	}

	c, rec = newServer(t, 200, checkoutQuoteJSON)
	if _, err := c.QuoteCheckoutSession(ctx(), "cs_1", CheckoutQuoteParams{SrcChain: "solana", SrcToken: "SOL", Sender: "9W", SlippageBps: 100}); err != nil {
		t.Fatal(err)
	}
	var b map[string]any
	json.Unmarshal([]byte(rec.body), &b)
	if b["slippageBps"] != float64(100) || b["srcChain"] != "solana" {
		t.Fatalf("body: %s", rec.body)
	}
}

func TestTestModeFields(t *testing.T) {
	c, _ := newServer(t, 200, `{"best":{"quoteId":"q_1","provider":"sandbox","dstAmount":"1","dstAmountMin":"1","estimatedSeconds":1,"fees":{"ourFeeBps":20},"expiresAt":1},"quotes":[],"errors":[],"livemode":false}`)
	q, err := c.Quote(ctx(), QuoteParams{SrcChain: "base", SrcToken: "USDC", Amount: "100004001", DstChain: "solana", DstToken: "SOL", Sender: "a", Recipient: "b"})
	if err != nil || q.Livemode == nil || *q.Livemode || q.Best.Provider != "sandbox" {
		t.Fatalf("%+v %v", q, err)
	}
	c, _ = newServer(t, 200, quoteJSON)
	if q, _ = c.Quote(ctx(), QuoteParams{}); q.Livemode != nil {
		t.Fatalf("livemode should be nil when absent")
	}

	c, _ = newServer(t, 200, `{"tx":{"vm":"evm","chainId":8453,"to":"0x","data":"0x","value":"0","sandbox":true}}`)
	tx, err := c.BuildTx(ctx(), "q_1")
	if err != nil || !tx.Sandbox {
		t.Fatalf("%+v %v", tx, err)
	}

	c, _ = newServer(t, 200, `{"orders":[{"quoteId":"q_1","livemode":false,"checkoutSessionId":"cs_1"}]}`)
	o, err := c.ListOrders(ctx(), ListOrdersParams{Address: "a"})
	if err != nil || o.Orders[0].Livemode == nil || *o.Orders[0].Livemode || o.Orders[0].CheckoutSessionID != "cs_1" {
		t.Fatalf("%+v %v", o, err)
	}
}
