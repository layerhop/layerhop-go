package layerhop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const depositAddressJSON = `{"id":"da_01m5f3k2c4x8q6r0v3b5n7p9d","object":"deposit_address","livemode":false,"status":"active","provider":"sandbox",` +
	`"address":"0x4c3f1a0b9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a","chainId":8453,"token":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","dstChainId":42161,` +
	`"dstToken":"0xaf88d065e77c8cC2239327C5EDb3A432268e5831","recipient":"0x2222222222222222222222222222222222222222","refundTo":"0x1111111111111111111111111111111111111111",` +
	`"reusable":true,"amount":null,"fees":{"ourFeeBps":20,"integratorFeeBps":0,"integrator":null},"label":"user_42","metadata":{"userId":"42"},` +
	`"depositCount":0,"lastDepositAt":null,"createdAt":"2026-10-19T10:00:00.000Z"}`

const depositOrderJSON = `{"quoteId":"dep_5f0c1e2d","provider":"sandbox","status":"submitted","statusDetail":null,"channel":"api","ourFeeBps":20,"partnerFeeBps":0,` +
	`"sender":"0x1111111111111111111111111111111111111111","recipient":"0x2222222222222222222222222222222222222222","srcChainId":8453,"srcToken":"0x83","dstChainId":42161,` +
	`"dstToken":"0xaf","amount":"25000000","dstAmount":"24930000","srcTxHash":"0xabc","dstTxHash":null,"livemode":false,"checkoutSessionId":null,"refundId":null,` +
	`"depositAddressId":"da_01m5f3k2c4x8q6r0v3b5n7p9d","keyId":"key_1","partnerId":null,"partnerFeeEstimate":"0","createdAt":"t","updatedAt":"t"}`

func withExtra(obj, extra string) string { return strings.TrimSuffix(obj, "}") + "," + extra + "}" }

func TestCreateDepositAddress(t *testing.T) {
	resp := withExtra(depositAddressJSON, `"estimate":{"amount":"100000000","dstAmount":"99720000","dstAmountMin":"99221400","estimatedSeconds":12}`)
	c, rec := newServer(t, 201, resp)
	d, err := c.CreateDepositAddress(ctx(), CreateDepositAddressParams{
		SrcChain: "base", SrcToken: "USDC", DstChain: "42161", DstToken: "USDC", Recipient: "0x2222", RefundTo: "0x1111",
		Reusable: ptr(true), Amount: "100000000", Label: "user_42", Metadata: map[string]string{"userId": "42"},
		Integrator: "acme", IntegratorFeeBps: ptr(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/deposit-addresses" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"srcChain":"base","srcToken":"USDC","dstChain":"42161","dstToken":"USDC","recipient":"0x2222","refundTo":"0x1111","reusable":true,"amount":"100000000","label":"user_42","metadata":{"userId":"42"},"integrator":"acme","integratorFeeBps":10}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if d.ID != "da_01m5f3k2c4x8q6r0v3b5n7p9d" || d.Object != "deposit_address" || d.Status != DepositAddressActive || d.Provider != "sandbox" || d.Livemode ||
		d.ChainID != 8453 || d.DstChainID != 42161 || !d.Reusable || d.Amount != "" || d.Fees.OurFeeBps != 20 || d.Fees.Integrator != "" ||
		d.Label != "user_42" || d.Metadata["userId"] != "42" || d.LastDepositAt != "" || d.Estimate == nil || d.Estimate.DstAmountMin != "99221400" ||
		d.Estimate.EstimatedSeconds != 12 || d.Deposits != nil {
		t.Fatalf("deposit address: %+v", d)
	}

	// Minimal body; the client's Integrator / IntegratorFeeBps are the defaults.
	c, rec = newServer(t, 201, resp)
	if _, err := c.CreateDepositAddress(ctx(), CreateDepositAddressParams{SrcChain: "8453", SrcToken: "USDC", DstChain: "arbitrum", DstToken: "USDC", Recipient: "0x2", RefundTo: "0x1"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"srcChain":"8453","srcToken":"USDC","dstChain":"arbitrum","dstToken":"USDC","recipient":"0x2","refundTo":"0x1"}` {
		t.Fatalf("minimal body: %s", rec.body)
	}
	c, rec = newServer(t, 201, resp)
	c.opts.Integrator, c.opts.IntegratorFeeBps = "acme", ptr(5)
	if _, err := c.CreateDepositAddress(ctx(), CreateDepositAddressParams{SrcChain: "base", SrcToken: "USDC", DstChain: "arbitrum", DstToken: "USDC", Recipient: "0x2", RefundTo: "0x1", Reusable: ptr(false), Amount: "5000000"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"srcChain":"base","srcToken":"USDC","dstChain":"arbitrum","dstToken":"USDC","recipient":"0x2","refundTo":"0x1","reusable":false,"amount":"5000000","integrator":"acme","integratorFeeBps":5}` {
		t.Fatalf("defaults body: %s", rec.body)
	}

	// Live mode disabled by the operator.
	c, _ = newServer(t, 403, `{"error":{"code":"FORBIDDEN","message":"Deposit addresses are not enabled for live mode yet.","requestId":"r1"}}`)
	_, err = c.CreateDepositAddress(ctx(), CreateDepositAddressParams{SrcChain: "base", SrcToken: "USDC", DstChain: "arbitrum", DstToken: "USDC", Recipient: "0x2", RefundTo: "0x1"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" || ae.Status != 403 {
		t.Fatalf("%v", err)
	}
}

func TestGetListDepositAddresses(t *testing.T) {
	c, rec := newServer(t, 200, withExtra(strings.Replace(depositAddressJSON, `"depositCount":0`, `"depositCount":1`, 1), `"deposits":[`+depositOrderJSON+`]`))
	d, err := c.GetDepositAddress(ctx(), "da_1")
	if err != nil || rec.method != "GET" || rec.path != "/v1/deposit-addresses/da_1" {
		t.Fatalf("get: %+v %v %+v", d, err, rec)
	}
	if d.DepositCount != 1 || len(d.Deposits) != 1 || d.Deposits[0].DepositAddressID != "da_01m5f3k2c4x8q6r0v3b5n7p9d" || d.Deposits[0].Amount != "25000000" || d.Estimate != nil {
		t.Fatalf("deposits: %+v", d)
	}
	c, rec = newServer(t, 200, withExtra(depositAddressJSON, `"deposits":[]`))
	if _, err := c.GetDepositAddress(ctx(), "a/b"); err != nil || rec.rawPath != "/v1/deposit-addresses/a%2Fb" {
		t.Fatalf("encoded: %s %v", rec.rawPath, err)
	}

	c, rec = newServer(t, 200, `{"depositAddresses":[`+depositAddressJSON+`],"nextCursor":"c2"}`)
	r, err := c.ListDepositAddresses(ctx(), ListDepositAddressesParams{Status: DepositAddressActive, Label: "user_42", Cursor: "c1", Limit: 10})
	if err != nil || len(r.DepositAddresses) != 1 || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/deposit-addresses" || rec.query != "cursor=c1&label=user_42&limit=10&status=active" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"depositAddresses":[],"nextCursor":null}`)
	r, err = c.ListDepositAddresses(ctx(), ListDepositAddressesParams{})
	if err != nil || rec.query != "" || r.NextCursor != "" || len(r.DepositAddresses) != 0 {
		t.Fatalf("empty: %s %+v %v", rec.query, r, err)
	}
}

func TestRefreshDisableSimulateDeposit(t *testing.T) {
	c, rec := newServer(t, 202, depositAddressJSON)
	if d, err := c.RefreshDepositAddress(ctx(), "da_1"); err != nil || d.Status != DepositAddressActive || rec.method != "POST" || rec.path != "/v1/deposit-addresses/da_1/refresh" || rec.body != `{}` {
		t.Fatalf("refresh: %+v %v %+v", d, err, rec)
	}
	c, rec = newServer(t, 200, strings.Replace(depositAddressJSON, `"status":"active"`, `"status":"disabled"`, 1))
	if d, err := c.DisableDepositAddress(ctx(), "da_1"); err != nil || d.Status != DepositAddressDisabled || rec.method != "POST" || rec.path != "/v1/deposit-addresses/da_1/disable" || rec.body != `{}` {
		t.Fatalf("disable: %+v %v %+v", d, err, rec)
	}
	c, rec = newServer(t, 201, `{"order":`+depositOrderJSON+`}`)
	s, err := c.SimulateDeposit(ctx(), "da_1", "25000000")
	if err != nil || rec.method != "POST" || rec.path != "/v1/deposit-addresses/da_1/simulate" || rec.body != `{"amount":"25000000"}` {
		t.Fatalf("simulate: %+v %v %+v", s, err, rec)
	}
	if s.Order.Provider != "sandbox" || s.Order.DepositAddressID != "da_01m5f3k2c4x8q6r0v3b5n7p9d" || s.Order.Livemode == nil || *s.Order.Livemode {
		t.Fatalf("order: %+v", s.Order)
	}
	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"This deposit address is disabled","requestId":"r2"}}`)
	_, err = c.RefreshDepositAddress(ctx(), "da_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}

func TestOrderDepositAddressID(t *testing.T) {
	var o Order
	if err := json.Unmarshal([]byte(depositOrderJSON), &o); err != nil || o.DepositAddressID != "da_01m5f3k2c4x8q6r0v3b5n7p9d" || o.CheckoutSessionID != "" {
		t.Fatalf("%+v %v", o, err)
	}
}
