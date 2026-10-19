package layerhop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const subscriptionJSON = `{"id":"sub_01k7v0c2c4x8q6r0v3b5n7p9d","object":"subscription","livemode":true,"status":"active","customerEmail":"ada@example.com",` +
	`"customerName":"Ada Lovelace","merchantName":"Studio Nine","description":"Pro plan","memo":null,"productId":null,"amount":"49000000",` +
	`"settlement":{"chainId":8453,"token":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","symbol":"USDC","decimals":6},` +
	`"recipient":"0x2222222222222222222222222222222222222222","interval":"month","intervalCount":1,"daysUntilDue":3,` +
	`"currentPeriodStart":"2026-10-18T10:00:00.000Z","currentPeriodEnd":"2026-11-18T10:00:00.000Z","nextInvoiceAt":"2026-11-18T10:00:00.000Z",` +
	`"cancelAt":null,"cancelAtPeriodEnd":false,"canceledAt":null,"cancellationReason":null,"pausedAt":null,` +
	`"latestInvoiceId":"inv_01k7v0c2c4x8q6r0v3b5n7p9d","metadata":{"plan":"pro"},"createdAt":"2026-10-18T10:00:00.000Z","updatedAt":"2026-10-18T10:00:00.000Z"}`

func TestCreateSubscription(t *testing.T) {
	c, rec := newServer(t, 201, subscriptionJSON)
	s, err := c.CreateSubscription(ctx(), CreateSubscriptionParams{
		CustomerEmail: "ada@example.com", Interval: IntervalMonth, IntervalCount: 1,
		Amount: "49000000", SettleChain: "base", SettleToken: "USDC", Recipient: "0x2222",
		Description: "Pro plan", CustomerName: "Ada Lovelace", DaysUntilDue: ptr(0), Metadata: map[string]string{"plan": "pro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/subscriptions" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"customerEmail":"ada@example.com","interval":"month","intervalCount":1,"amount":"49000000","settleChain":"base","settleToken":"USDC","recipient":"0x2222","description":"Pro plan","customerName":"Ada Lovelace","daysUntilDue":0,"metadata":{"plan":"pro"}}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if s.ID != "sub_01k7v0c2c4x8q6r0v3b5n7p9d" || s.Object != "subscription" || !s.Livemode || s.Status != SubscriptionActive || s.Memo != "" || s.ProductID != "" ||
		s.Settlement.Symbol != "USDC" || s.Interval != IntervalMonth || s.IntervalCount != 1 || s.DaysUntilDue != 3 || s.NextInvoiceAt != "2026-11-18T10:00:00.000Z" ||
		s.CancelAt != "" || s.CancelAtPeriodEnd || s.LatestInvoiceID != "inv_01k7v0c2c4x8q6r0v3b5n7p9d" || s.Metadata["plan"] != "pro" {
		t.Fatalf("subscription: %+v", s)
	}

	// From a product: only the fields that are set.
	c, rec = newServer(t, 201, subscriptionJSON)
	if _, err := c.CreateSubscription(ctx(), CreateSubscriptionParams{CustomerEmail: "a@b.co", Interval: IntervalYear, ProductID: "prod_1", StartAt: "2026-11-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"customerEmail":"a@b.co","interval":"year","productId":"prod_1","startAt":"2026-11-01T00:00:00Z"}` {
		t.Fatalf("product body: %s", rec.body)
	}

	c, _ = newServer(t, 400, `{"error":{"code":"INVALID_PARAMS","message":"description is required without productId","requestId":"r1"}}`)
	_, err = c.CreateSubscription(ctx(), CreateSubscriptionParams{CustomerEmail: "a@b.co", Interval: IntervalMonth})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "INVALID_PARAMS" || ae.Status != 400 {
		t.Fatalf("%v", err)
	}
}

func TestGetListUpdateSubscriptions(t *testing.T) {
	c, rec := newServer(t, 200, subscriptionJSON)
	s, err := c.GetSubscription(ctx(), "sub_1")
	if err != nil || s.CustomerEmail != "ada@example.com" || rec.method != "GET" || rec.path != "/v1/subscriptions/sub_1" {
		t.Fatalf("get: %+v %v %+v", s, err, rec)
	}
	c, rec = newServer(t, 200, subscriptionJSON)
	if _, err := c.GetSubscription(ctx(), "a/b"); err != nil || rec.rawPath != "/v1/subscriptions/a%2Fb" {
		t.Fatalf("encoded: %s %v", rec.rawPath, err)
	}

	c, rec = newServer(t, 200, `{"subscriptions":[`+subscriptionJSON+`],"nextCursor":"c2"}`)
	r, err := c.ListSubscriptions(ctx(), ListSubscriptionsParams{Status: SubscriptionPastDue, CustomerEmail: "ada@example.com", Cursor: "c1", Limit: 10})
	if err != nil || len(r.Subscriptions) != 1 || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/subscriptions" || rec.query != "cursor=c1&customerEmail=ada%40example.com&limit=10&status=past_due" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"subscriptions":[],"nextCursor":null}`)
	r, err = c.ListSubscriptions(ctx(), ListSubscriptionsParams{})
	if err != nil || rec.query != "" || r.NextCursor != "" || len(r.Subscriptions) != 0 {
		t.Fatalf("empty: %s %+v %v", rec.query, r, err)
	}

	c, rec = newServer(t, 200, subscriptionJSON)
	if _, err := c.UpdateSubscription(ctx(), "sub_1", UpdateSubscriptionParams{
		Amount: ptr("59000000"), Memo: ptr(""), DaysUntilDue: ptr(7), CancelAtPeriodEnd: ptr(false), Metadata: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	if rec.method != "PATCH" || rec.path != "/v1/subscriptions/sub_1" {
		t.Fatalf("req: %+v", rec)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(rec.body), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 || body["amount"] != "59000000" || body["memo"] != nil || body["daysUntilDue"] != float64(7) || body["cancelAtPeriodEnd"] != false {
		t.Fatalf("patch body: %s", rec.body)
	}
	if _, ok := body["metadata"]; !ok {
		t.Fatalf("metadata null missing: %s", rec.body)
	}
	c, rec = newServer(t, 200, subscriptionJSON)
	if _, err := c.UpdateSubscription(ctx(), "sub_1", UpdateSubscriptionParams{}); err != nil || rec.body != `{}` {
		t.Fatalf("empty patch: %s %v", rec.body, err)
	}
}

func TestCancelPauseResumeSubscription(t *testing.T) {
	c, rec := newServer(t, 200, strings.Replace(subscriptionJSON, `"cancelAtPeriodEnd":false`, `"cancelAtPeriodEnd":true`, 1))
	s, err := c.CancelSubscription(ctx(), "sub_1", true)
	if err != nil || !s.CancelAtPeriodEnd || rec.method != "POST" || rec.path != "/v1/subscriptions/sub_1/cancel" || rec.body != `{"atPeriodEnd":true}` {
		t.Fatalf("cancel at period end: %+v %v %+v", s, err, rec)
	}
	canceled := strings.NewReplacer(`"status":"active"`, `"status":"canceled"`, `"cancellationReason":null`, `"cancellationReason":"requested"`).Replace(subscriptionJSON)
	c, rec = newServer(t, 200, canceled)
	s, err = c.CancelSubscription(ctx(), "sub_1", false)
	if err != nil || s.Status != SubscriptionCanceled || s.CancellationReason != "requested" || rec.body != `{"atPeriodEnd":false}` {
		t.Fatalf("cancel now: %+v %v %s", s, err, rec.body)
	}

	c, rec = newServer(t, 200, strings.Replace(subscriptionJSON, `"status":"active"`, `"status":"paused"`, 1))
	if s, err := c.PauseSubscription(ctx(), "sub_1"); err != nil || s.Status != SubscriptionPaused || rec.method != "POST" || rec.path != "/v1/subscriptions/sub_1/pause" || rec.body != `{}` {
		t.Fatalf("pause: %+v %v %+v", s, err, rec)
	}
	c, rec = newServer(t, 200, subscriptionJSON)
	if s, err := c.ResumeSubscription(ctx(), "sub_1"); err != nil || s.Status != SubscriptionActive || rec.method != "POST" || rec.path != "/v1/subscriptions/sub_1/resume" || rec.body != `{}` {
		t.Fatalf("resume: %+v %v %+v", s, err, rec)
	}

	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"This subscription is canceled","requestId":"r2"}}`)
	_, err = c.PauseSubscription(ctx(), "sub_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}

func TestListSubscriptionInvoices(t *testing.T) {
	inv := strings.Replace(invoiceJSON, `"sessionId":null`, `"sessionId":null,"subscriptionId":"sub_01k7v0c2c4x8q6r0v3b5n7p9d"`, 1)
	c, rec := newServer(t, 200, `{"invoices":[`+inv+`]}`)
	list, err := c.ListSubscriptionInvoices(ctx(), "sub_1")
	if err != nil || len(list) != 1 || list[0].SubscriptionID != "sub_01k7v0c2c4x8q6r0v3b5n7p9d" || rec.method != "GET" || rec.path != "/v1/subscriptions/sub_1/invoices" {
		t.Fatalf("invoices: %+v %v %+v", list, err, rec)
	}
	var plain Invoice
	if err := json.Unmarshal([]byte(invoiceJSON), &plain); err != nil || plain.SubscriptionID != "" {
		t.Fatalf("%+v %v", plain, err)
	}
}

func TestSubscriptionWebhookEvent(t *testing.T) {
	body := `{"id":"evt_1","type":"subscription.renewed","livemode":true,"createdAt":"t","data":{"subscription":` + subscriptionJSON + `,"invoice":` + invoiceJSON + `}}`
	var e WebhookEvent
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != EventSubscriptionRenewed || e.Data.Subscription == nil || e.Data.Subscription.ID != "sub_01k7v0c2c4x8q6r0v3b5n7p9d" || e.Data.Invoice == nil {
		t.Fatalf("%+v", e)
	}
}
