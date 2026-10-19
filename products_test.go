package layerhop

import (
	"errors"
	"strings"
	"testing"
)

const productJSON = `{"id":"prod_01m57hq2c4x8q6r0v3b5n7p9d","object":"checkout.product","livemode":true,"active":true,"name":"Pro plan","description":"1 month","merchantName":null,"amount":"49000000",` +
	`"settlement":{"chainId":8453,"token":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","symbol":"USDC","decimals":6},"url":"https://layerhop.com/buy/prod_01m57hq2c4x8q6r0v3b5n7p9d",` +
	`"recipient":"0x2222222222222222222222222222222222222222","successUrl":"https://shop.example.com/thanks","cancelUrl":null,"expiresInMinutes":60,"allowedOrigins":["https://shop.example.com"],` +
	`"createdAt":"2026-10-18T10:00:00.000Z","updatedAt":"2026-10-18T10:00:00.000Z"}`

func ptr[T any](v T) *T { return &v }

func TestVersion(t *testing.T) {
	if Version != "0.6.0" {
		t.Fatalf("version %s", Version)
	}
}

func TestCreateCheckoutProduct(t *testing.T) {
	c, rec := newServer(t, 201, productJSON)
	p, err := c.CreateCheckoutProduct(ctx(), CreateCheckoutProductParams{
		Name: "Pro plan", Description: "1 month", Amount: "49000000", SettleChain: "base", SettleToken: "USDC", Recipient: "0x2222",
		SuccessURL: "https://shop.example.com/thanks", ExpiresInMinutes: 60, AllowedOrigins: []string{"https://shop.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/checkout/products" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"name":"Pro plan","description":"1 month","amount":"49000000","settleChain":"base","settleToken":"USDC","recipient":"0x2222","successUrl":"https://shop.example.com/thanks","expiresInMinutes":60,"allowedOrigins":["https://shop.example.com"]}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if p.ID != "prod_01m57hq2c4x8q6r0v3b5n7p9d" || p.Object != "checkout.product" || !p.Livemode || !p.Active || p.Name != "Pro plan" {
		t.Fatalf("product: %+v", p)
	}
	if p.Settlement.Symbol != "USDC" || p.URL != "https://layerhop.com/buy/prod_01m57hq2c4x8q6r0v3b5n7p9d" || p.MerchantName != "" || p.CancelURL != "" ||
		p.ExpiresInMinutes != 60 || len(p.AllowedOrigins) != 1 || p.Recipient == "" || p.CreatedAt == "" {
		t.Fatalf("product fields: %+v", p)
	}

	c, rec = newServer(t, 201, productJSON)
	if _, err := c.CreateCheckoutProduct(ctx(), CreateCheckoutProductParams{Name: "Tip", Amount: "1", SettleChain: "8453", SettleToken: "USDC", Recipient: "0x2", Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"name":"Tip","amount":"1","settleChain":"8453","settleToken":"USDC","recipient":"0x2","active":false}` {
		t.Fatalf("minimal body: %s", rec.body)
	}
}

func TestGetCheckoutProduct(t *testing.T) {
	c, rec := newServer(t, 200, `{"id":"prod_1","object":"checkout.product","livemode":false,"active":true,"name":"Tip","description":null,"merchantName":null,"amount":"1","settlement":{"chainId":8453,"token":"0x83","symbol":"USDC","decimals":6},"url":"u"}`)
	p, err := c.GetCheckoutProduct(ctx(), "prod_1")
	if err != nil || p.Livemode || p.Recipient != "" || p.AllowedOrigins != nil || p.Description != "" {
		t.Fatalf("%+v %v", p, err)
	}
	if rec.method != "GET" || rec.path != "/v1/checkout/products/prod_1" || rec.body != "" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, productJSON)
	if _, err := c.GetCheckoutProduct(ctx(), "a/b"); err != nil || rec.rawPath != "/v1/checkout/products/a%2Fb" {
		t.Fatalf("encoded: %s %v", rec.rawPath, err)
	}
}

func TestListCheckoutProducts(t *testing.T) {
	c, rec := newServer(t, 200, `{"products":[`+productJSON+`],"nextCursor":"c2"}`)
	r, err := c.ListCheckoutProducts(ctx(), ListCheckoutProductsParams{Active: ptr(true), Cursor: "c1", Limit: 10})
	if err != nil || len(r.Products) != 1 || r.Products[0].Amount != "49000000" || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/checkout/products" || rec.query != "active=true&cursor=c1&limit=10" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"products":[],"nextCursor":null}`)
	if _, err := c.ListCheckoutProducts(ctx(), ListCheckoutProductsParams{Active: ptr(false)}); err != nil || rec.query != "active=false" {
		t.Fatalf("inactive: %s %v", rec.query, err)
	}
	c, rec = newServer(t, 200, `{"products":[],"nextCursor":null}`)
	r, err = c.ListCheckoutProducts(ctx(), ListCheckoutProductsParams{})
	if err != nil || rec.query != "" || r.NextCursor != "" || len(r.Products) != 0 {
		t.Fatalf("empty params: %s %+v %v", rec.query, r, err)
	}
}

func TestUpdateCheckoutProduct(t *testing.T) {
	c, rec := newServer(t, 200, strings.Replace(productJSON, `"active":true`, `"active":false`, 1))
	p, err := c.UpdateCheckoutProduct(ctx(), "prod_1", UpdateCheckoutProductParams{Active: ptr(false)})
	if err != nil || p.Active {
		t.Fatalf("%+v %v", p, err)
	}
	if rec.method != "PATCH" || rec.path != "/v1/checkout/products/prod_1" || rec.body != `{"active":false}` || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}

	c, rec = newServer(t, 200, productJSON)
	_, err = c.UpdateCheckoutProduct(ctx(), "prod_1", UpdateCheckoutProductParams{
		Name: ptr("Pro"), Description: ptr(""), CancelURL: ptr(""), Amount: ptr("50000000"), SettleChain: ptr("42161"),
		ExpiresInMinutes: ptr(30), AllowedOrigins: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"allowedOrigins":[],"amount":"50000000","cancelUrl":null,"description":null,"expiresInMinutes":30,"name":"Pro","settleChain":"42161"}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}

	c, rec = newServer(t, 200, productJSON)
	if _, err := c.UpdateCheckoutProduct(ctx(), "prod_1", UpdateCheckoutProductParams{}); err != nil || rec.body != `{}` {
		t.Fatalf("empty update: %s %v", rec.body, err)
	}
}

func TestStartProductCheckout(t *testing.T) {
	resp := strings.Replace(sessionJSON, `"successUrl"`, `"productId":"prod_1","successUrl"`, 1)
	c, rec := newServer(t, 201, resp)
	s, err := c.StartProductCheckout(ctx(), "prod_1", StartProductCheckoutParams{Reference: "order_1042", Metadata: map[string]string{"plan": "pro"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/checkout/products/prod_1/sessions" || rec.body != `{"reference":"order_1042","metadata":{"plan":"pro"}}` {
		t.Fatalf("req: %+v", rec)
	}
	if s.Object != "checkout.session" || s.ProductID != "prod_1" || s.Amount != "49000000" {
		t.Fatalf("session: %+v", s)
	}

	c, rec = newServer(t, 201, sessionJSON)
	s, err = c.StartProductCheckout(ctx(), "prod_1", StartProductCheckoutParams{})
	if err != nil || rec.body != `{}` || s.ProductID != "" {
		t.Fatalf("empty: %s %+v %v", rec.body, s, err)
	}

	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"Product is inactive","requestId":"r1"}}`)
	_, err = c.StartProductCheckout(ctx(), "prod_1", StartProductCheckoutParams{})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}
