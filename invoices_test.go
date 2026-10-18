package layerhop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const invoiceJSON = `{"id":"inv_01m57hq2c4x8q6r0v3b5n7p9d","object":"invoice","livemode":true,"number":"INV-0001","status":"open","merchantName":"Acme","customerName":"Jane","memo":null,` +
	`"lineItems":[{"description":"Design work","quantity":2,"unitAmount":"100000000"},{"description":"Hosting","quantity":1,"unitAmount":"25000000"}],"amount":"225000000",` +
	`"settlement":{"chainId":8453,"token":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","symbol":"USDC","decimals":6},"dueAt":"2026-11-01T23:59:59.000Z","paidAt":null,` +
	`"url":"https://layerhop.com/invoice/inv_01m57hq2c4x8q6r0v3b5n7p9d","receiptUrl":null,"createdAt":"2026-10-18T10:00:00.000Z","customerEmail":"jane@example.com",` +
	`"recipient":"0x2222222222222222222222222222222222222222","sessionId":null,"sentAt":"2026-10-18T10:00:00.000Z","receiptSentAt":null,"voidedAt":null,"updatedAt":"2026-10-18T10:00:00.000Z"}`

const refundJSON = `{"id":"re_01m57hq2c4x8q6r0v3b5n7p9d","object":"refund","sessionId":"cs_1","livemode":true,"status":"pending","statusDetail":null,"amount":"10000000","reason":"Damaged",` +
	`"quoteId":"q_r1","to":{"chainId":42161,"token":"0x0000000000000000000000000000000000000000","address":"0x1111111111111111111111111111111111111111"},"txHash":null,` +
	`"createdAt":"2026-10-18T12:00:00.000Z","updatedAt":"2026-10-18T12:00:00.000Z"}`

func TestCreateInvoice(t *testing.T) {
	c, rec := newServer(t, 201, invoiceJSON)
	i, err := c.CreateInvoice(ctx(), CreateInvoiceParams{
		CustomerEmail: "jane@example.com", CustomerName: "Jane", MerchantName: "Acme",
		LineItems:   []InvoiceLineItem{{Description: "Design work", Quantity: 2, UnitAmount: "100000000"}, {Description: "Hosting", Quantity: 1, UnitAmount: "25000000"}},
		SettleChain: "base", SettleToken: "USDC", Recipient: "0x2222", DueAt: "2026-11-01", Send: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/invoices" || rec.header.Get("x-api-key") != "k1" || rec.header.Get("content-type") != "application/json" {
		t.Fatalf("req: %+v", rec)
	}
	want := `{"customerEmail":"jane@example.com","customerName":"Jane","merchantName":"Acme","lineItems":[{"description":"Design work","quantity":2,"unitAmount":"100000000"},{"description":"Hosting","quantity":1,"unitAmount":"25000000"}],"settleChain":"base","settleToken":"USDC","recipient":"0x2222","dueAt":"2026-11-01","send":true}`
	if rec.body != want {
		t.Fatalf("body\n got %s\nwant %s", rec.body, want)
	}
	if i.ID != "inv_01m57hq2c4x8q6r0v3b5n7p9d" || i.Object != "invoice" || i.Status != InvoiceOpen || i.Number != "INV-0001" || i.Amount != "225000000" ||
		len(i.LineItems) != 2 || i.LineItems[0].Quantity != 2 || i.Settlement.Symbol != "USDC" || i.Memo != "" || i.SessionID != "" ||
		i.URL != "https://layerhop.com/invoice/inv_01m57hq2c4x8q6r0v3b5n7p9d" || i.CustomerEmail != "jane@example.com" || i.SentAt == "" {
		t.Fatalf("invoice: %+v", i)
	}

	c, rec = newServer(t, 201, invoiceJSON)
	if _, err := c.CreateInvoice(ctx(), CreateInvoiceParams{LineItems: []InvoiceLineItem{{Description: "x", Quantity: 1, UnitAmount: "1"}}, SettleChain: "8453", SettleToken: "USDC", Recipient: "0x2"}); err != nil {
		t.Fatal(err)
	}
	if rec.body != `{"lineItems":[{"description":"x","quantity":1,"unitAmount":"1"}],"settleChain":"8453","settleToken":"USDC","recipient":"0x2"}` {
		t.Fatalf("minimal body: %s", rec.body)
	}
}

func TestGetListInvoices(t *testing.T) {
	c, rec := newServer(t, 200, invoiceJSON)
	if i, err := c.GetInvoice(ctx(), "inv_1"); err != nil || i.Number != "INV-0001" || rec.method != "GET" || rec.path != "/v1/invoices/inv_1" {
		t.Fatalf("get: %+v %v %+v", i, err, rec)
	}
	c, rec = newServer(t, 200, invoiceJSON)
	if _, err := c.GetInvoice(ctx(), "a/b"); err != nil || rec.rawPath != "/v1/invoices/a%2Fb" {
		t.Fatalf("encoded: %s %v", rec.rawPath, err)
	}

	c, rec = newServer(t, 200, `{"invoices":[`+invoiceJSON+`],"nextCursor":"c2"}`)
	r, err := c.ListInvoices(ctx(), ListInvoicesParams{Status: InvoicePaid, Cursor: "c1", Limit: 10})
	if err != nil || len(r.Invoices) != 1 || r.NextCursor != "c2" {
		t.Fatalf("%+v %v", r, err)
	}
	if rec.method != "GET" || rec.path != "/v1/invoices" || rec.query != "cursor=c1&limit=10&status=paid" {
		t.Fatalf("req: %+v", rec)
	}
	c, rec = newServer(t, 200, `{"invoices":[],"nextCursor":null}`)
	r, err = c.ListInvoices(ctx(), ListInvoicesParams{})
	if err != nil || rec.query != "" || r.NextCursor != "" || len(r.Invoices) != 0 {
		t.Fatalf("empty: %s %+v %v", rec.query, r, err)
	}
}

func TestUpdateInvoice(t *testing.T) {
	c, rec := newServer(t, 200, strings.Replace(invoiceJSON, `"status":"open"`, `"status":"draft"`, 1))
	i, err := c.UpdateInvoice(ctx(), "inv_1", UpdateInvoiceParams{
		Memo: ptr("Thanks!"), DueAt: ptr(""), CustomerName: ptr(""), SettleChain: ptr("arbitrum"),
		LineItems: []InvoiceLineItem{{Description: "Audit", Quantity: 1, UnitAmount: "5"}},
	})
	if err != nil || i.Status != InvoiceDraft {
		t.Fatalf("%+v %v", i, err)
	}
	want := `{"customerName":null,"dueAt":null,"lineItems":[{"description":"Audit","quantity":1,"unitAmount":"5"}],"memo":"Thanks!","settleChain":"arbitrum"}`
	if rec.method != "PATCH" || rec.path != "/v1/invoices/inv_1" || rec.body != want {
		t.Fatalf("req: %+v\nwant %s", rec, want)
	}
	c, rec = newServer(t, 200, invoiceJSON)
	if _, err := c.UpdateInvoice(ctx(), "inv_1", UpdateInvoiceParams{}); err != nil || rec.body != `{}` {
		t.Fatalf("empty update: %s %v", rec.body, err)
	}
}

func TestSendVoidInvoice(t *testing.T) {
	c, rec := newServer(t, 200, invoiceJSON)
	if i, err := c.SendInvoice(ctx(), "inv_1"); err != nil || i.Status != InvoiceOpen || rec.method != "POST" || rec.path != "/v1/invoices/inv_1/send" || rec.body != `{}` {
		t.Fatalf("send: %+v %v %+v", i, err, rec)
	}
	c, rec = newServer(t, 200, strings.Replace(invoiceJSON, `"voidedAt":null`, `"voidedAt":"2026-10-19T00:00:00.000Z"`, 1))
	if i, err := c.VoidInvoice(ctx(), "inv_1"); err != nil || i.VoidedAt == "" || rec.method != "POST" || rec.path != "/v1/invoices/inv_1/void" {
		t.Fatalf("void: %+v %v %+v", i, err, rec)
	}
	c, _ = newServer(t, 409, `{"error":{"code":"CONFLICT","message":"This invoice is paid","requestId":"r1"}}`)
	_, err := c.VoidInvoice(ctx(), "inv_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "CONFLICT" || ae.Status != 409 {
		t.Fatalf("%v", err)
	}
}

func TestPublicInvoiceAndPay(t *testing.T) {
	public := `{"id":"inv_1","object":"invoice","livemode":false,"number":"INV-0001","status":"paid","merchantName":null,"customerName":null,"memo":null,"lineItems":[{"description":"x","quantity":1,"unitAmount":"1"}],` +
		`"amount":"1","settlement":{"chainId":8453,"token":"0x83","symbol":"USDC","decimals":6},"dueAt":null,"paidAt":"2026-10-18T11:00:00.000Z","url":"u","receiptUrl":"https://api.layerhop.com/v1/invoices/inv_1/receipt.pdf","createdAt":"2026-10-18T10:00:00.000Z"}`
	c, rec := newServer(t, 200, public)
	i, err := c.GetPublicInvoice(ctx(), "inv_1")
	if err != nil || i.Status != InvoicePaid || i.ReceiptURL == "" || i.Recipient != "" || i.CustomerEmail != "" || rec.method != "GET" || rec.path != "/v1/invoices/inv_1/public" {
		t.Fatalf("public: %+v %v %+v", i, err, rec)
	}
	c, rec = newServer(t, 201, `{"sessionId":"cs_test_1","url":"https://layerhop.com/pay/cs_test_1"}`)
	p, err := c.PayInvoice(ctx(), "inv_1")
	if err != nil || p.SessionID != "cs_test_1" || p.URL != "https://layerhop.com/pay/cs_test_1" || rec.method != "POST" || rec.path != "/v1/invoices/inv_1/pay" || rec.body != `{}` {
		t.Fatalf("pay: %+v %v %+v", p, err, rec)
	}
}

func TestCreateListRefunds(t *testing.T) {
	resp := `{"refund":` + refundJSON + `,"quote":{"quoteId":"q_r1","provider":"relay","srcAmount":"10000000","dstAmount":"3100000000000000","dstAmountMin":"3000000000000000","estimatedSeconds":20,"expiresAt":1760792400,"from":"0x2222"},"next":"Build the transaction..."}`
	c, rec := newServer(t, 201, resp)
	r, err := c.CreateRefund(ctx(), "cs_1", CreateRefundParams{Amount: "10000000", Reason: "Damaged", To: RefundToOriginal})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/checkout/sessions/cs_1/refunds" || rec.body != `{"amount":"10000000","reason":"Damaged","to":"original"}` {
		t.Fatalf("req: %+v", rec)
	}
	if r.Refund.ID != "re_01m57hq2c4x8q6r0v3b5n7p9d" || r.Refund.Status != RefundPending || r.Refund.To.ChainID != 42161 || r.Refund.TxHash != "" ||
		r.Quote.QuoteID != "q_r1" || r.Quote.From != "0x2222" || r.Quote.ExpiresAt != 1760792400 || r.Next == "" {
		t.Fatalf("refund: %+v", r)
	}
	c, rec = newServer(t, 201, resp)
	if _, err := c.CreateRefund(ctx(), "cs_1", CreateRefundParams{}); err != nil || rec.body != `{}` {
		t.Fatalf("full refund: %s %v", rec.body, err)
	}

	c, rec = newServer(t, 200, `{"refunds":[`+strings.Replace(refundJSON, `"pending"`, `"completed"`, 1)+`]}`)
	l, err := c.ListRefunds(ctx(), "cs_1")
	if err != nil || len(l.Refunds) != 1 || l.Refunds[0].Status != RefundCompleted || rec.method != "GET" || rec.path != "/v1/checkout/sessions/cs_1/refunds" {
		t.Fatalf("list: %+v %v %+v", l, err, rec)
	}
}

func TestSessionAndOrderNewFields(t *testing.T) {
	var s CheckoutSession
	if err := json.Unmarshal([]byte(strings.Replace(sessionJSON, `"successUrl"`, `"invoiceId":"inv_1","refundedAmount":"10000000","successUrl"`, 1)), &s); err != nil {
		t.Fatal(err)
	}
	if s.InvoiceID != "inv_1" || s.RefundedAmount != "10000000" {
		t.Fatalf("%+v", s)
	}
	var o Order
	if err := json.Unmarshal([]byte(`{"quoteId":"q_r1","refundId":"re_1"}`), &o); err != nil || o.RefundID != "re_1" {
		t.Fatalf("%+v %v", o, err)
	}
	var e WebhookEvent
	if err := json.Unmarshal([]byte(`{"id":"evt_1","type":"checkout.session.refunded","livemode":true,"createdAt":"t","data":{"session":`+sessionJSON+`,"refund":`+refundJSON+`}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != EventCheckoutSessionRefunded || e.Data.Session == nil || e.Data.Refund == nil || e.Data.Refund.Amount != "10000000" {
		t.Fatalf("%+v", e)
	}
	if err := json.Unmarshal([]byte(`{"id":"evt_2","type":"invoice.paid","livemode":true,"createdAt":"t","data":{"invoice":`+invoiceJSON+`,"session":null}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != EventInvoicePaid || e.Data.Invoice == nil || e.Data.Invoice.Number != "INV-0001" || EventInvoiceSent != "invoice.sent" {
		t.Fatalf("%+v", e)
	}
}

func TestServiceStatus(t *testing.T) {
	c, rec := newServer(t, 200, `{"status":"degraded","updatedAt":"2026-10-18T12:00:00.000Z","components":[{"id":"api","name":"API","group":"Core","status":"operational","latencyMs":42,"uptime90d":99.98,"days":[{"date":"2026-10-17","uptime":null},{"date":"2026-10-18","uptime":100}]},`+
		`{"id":"provider:relay","name":"Relay","group":"Liquidity providers","status":"degraded","latencyMs":null,"uptime90d":null,"days":[]}],`+
		`"incidents":[{"id":"inc_1","componentId":"provider:relay","component":"Relay","title":"Relay quotes failing","startedAt":"2026-10-18T11:00:00.000Z","resolvedAt":null}],"url":"https://layerhop.com/status"}`)
	s, err := c.ServiceStatus(ctx())
	if err != nil || rec.method != "GET" || rec.path != "/v1/status" {
		t.Fatalf("%v %+v", err, rec)
	}
	if s.Status != "degraded" || len(s.Components) != 2 || *s.Components[0].LatencyMs != 42 || *s.Components[0].Uptime90d != 99.98 || s.Components[0].Days[0].Uptime != nil ||
		*s.Components[0].Days[1].Uptime != 100 || s.Components[1].LatencyMs != nil || len(s.Incidents) != 1 || s.Incidents[0].ResolvedAt != "" || s.URL != "https://layerhop.com/status" {
		t.Fatalf("%+v", s)
	}
}
