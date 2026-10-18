package layerhop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseServer serves each chunk with a flush in between (and an optional pause), recording the
// request and whether the client hung up before the body was complete.
type sseServer struct {
	url       string
	rec       recorded
	hungUp    chan struct{}
	completed bool
}

func newSSEServer(t *testing.T, pause time.Duration, chunks ...string) (*Client, *sseServer) {
	t.Helper()
	s := &sseServer{hungUp: make(chan struct{}, 1)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.rec = recorded{r.Method, r.URL.Path, r.URL.EscapedPath(), r.URL.RawQuery, "", r.Header.Clone()}
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(200)
		f := w.(http.Flusher)
		for _, c := range chunks {
			select {
			case <-r.Context().Done():
				s.hungUp <- struct{}{}
				return
			case <-time.After(pause):
			}
			io.WriteString(w, c)
			f.Flush()
		}
		s.completed = true
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return New(Options{BaseURL: srv.URL, APIKey: "k1"}), s
}

// Comments, a ready frame, \r\n, a frame split across writes and a multi-line data field.
var orderChunks = []string{
	": connected\r\n\r\nevent: ready\r\ndata: {}\r\n\r\n",
	"event: order\r\ndata: {\"quoteId\":\"q_1\",\"status\":\"sub",
	"mitted\",\"provider\":\"relay\"}\r\n\r\n: ping\r\n\r",
	"\nevent: order\ndata: {\"quoteId\":\"q_1\",\ndata: \"status\":\"filled\",\"dstTxHash\":\"0xdef\"}\n\n",
	"event: other\ndata: {\"ignored\":true}\n\n",
}

func TestStreamOrder(t *testing.T) {
	c, s := newSSEServer(t, 0, orderChunks...)
	var got []Order
	for o, err := range c.StreamOrder(ctx(), "q/1") {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, *o)
	}
	if len(got) != 2 || got[0].Status != "submitted" || got[0].Provider != "relay" || got[1].Status != "filled" || got[1].DstTxHash != "0xdef" {
		t.Fatalf("orders: %+v", got)
	}
	if s.rec.method != "GET" || s.rec.rawPath != "/v1/orders/q%2F1/events" {
		t.Fatalf("req: %+v", s.rec)
	}
	if s.rec.header.Get("accept") != "text/event-stream" || s.rec.header.Get("x-api-key") != "k1" || !strings.HasPrefix(s.rec.header.Get("user-agent"), "layerhop-go/") {
		t.Fatalf("headers: %v", s.rec.header)
	}
}

func TestStreamCheckoutSession(t *testing.T) {
	paid := strings.Replace(sessionJSON, `"status":"processing"`, `"status":"paid"`, 1)
	c, s := newSSEServer(t, 0, "event: ready\ndata: {}\n\n", "event: session\ndata: "+sessionJSON+"\n\n", "event: session\ndata: "+paid+"\n\n")
	var statuses []string
	for cs, err := range c.StreamCheckoutSession(ctx(), "cs_1") {
		if err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, cs.Status)
		if cs.Settlement.Decimals != 6 {
			t.Fatalf("session: %+v", cs)
		}
	}
	if strings.Join(statuses, ",") != "processing,paid" || s.rec.path != "/v1/checkout/sessions/cs_1/events" {
		t.Fatalf("statuses %v, req %+v", statuses, s.rec)
	}
}

const eventJSON = `{"id":"evt_1","type":"order.filled","livemode":false,"sample":true,"createdAt":"2026-10-18T10:00:00.000Z","data":{"order":{"quoteId":"q_1","status":"filled"}}}`

func TestStreamWebhookEvents(t *testing.T) {
	second := `{"id":"evt_2","type":"checkout.session.completed","livemode":false,"createdAt":"c","data":{"session":` + sessionJSON + `}}`
	c, s := newSSEServer(t, 0, ": hi\n\nevent: ready\ndata: {}\n\n", "event: event\ndata: "+eventJSON+"\n\n", "event: event\r\ndata: "+second+"\r\n\r\n")
	var got []*WebhookEvent
	for e, err := range c.StreamWebhookEvents(ctx()) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	if len(got) != 2 || s.rec.path != "/v1/webhooks/stream" || s.rec.header.Get("x-api-key") != "k1" {
		t.Fatalf("events %d, req %+v", len(got), s.rec)
	}
	e := got[0]
	if e.ID != "evt_1" || e.Type != EventOrderFilled || e.Livemode || !e.Sample || e.Data.Order == nil || e.Data.Order.Status != "filled" || e.Data.Session != nil {
		t.Fatalf("event: %+v", e)
	}
	if string(e.Raw) != eventJSON {
		t.Fatalf("raw: %s", e.Raw)
	}
	if got[1].Data.Session == nil || got[1].Data.Session.Status != CheckoutProcessing {
		t.Fatalf("session event: %+v", got[1])
	}
}

func TestStreamBreakClosesConnection(t *testing.T) {
	chunks := []string{"event: order\ndata: {\"status\":\"pending\"}\n\n"}
	for i := 0; i < 50; i++ {
		chunks = append(chunks, ": ping\n\n")
	}
	c, s := newSSEServer(t, 10*time.Millisecond, chunks...)
	n := 0
	for o, err := range c.StreamOrder(ctx(), "q_1") {
		if err != nil || o.Status != "pending" {
			t.Fatal(o, err)
		}
		n++
		break
	}
	select {
	case <-s.hungUp:
	case <-time.After(2 * time.Second):
		t.Fatal("server never saw the client hang up")
	}
	if n != 1 || s.completed {
		t.Fatalf("n=%d completed=%v", n, s.completed)
	}
}

func TestStreamContextCancel(t *testing.T) {
	c, _ := newSSEServer(t, 50*time.Millisecond, "event: order\ndata: {\"status\":\"pending\"}\n\n", ": ping\n\n", ": ping\n\n", ": ping\n\n")
	cx, cancel := context.WithCancel(ctx())
	defer cancel()
	var errs []error
	for o, err := range c.StreamOrder(cx, "q_1") {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if o.Status == "pending" {
			cancel()
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], context.Canceled) {
		t.Fatalf("errs: %v", errs)
	}
}

func TestStreamIgnoresClientTimeout(t *testing.T) {
	// The HTTPClient's overall Timeout would cut a long-lived stream; streams rely on ctx instead.
	_, s := newSSEServer(t, 40*time.Millisecond, ": ping\n\n", ": ping\n\n", "event: order\ndata: {\"status\":\"filled\"}\n\n")
	hc := &http.Client{Timeout: 30 * time.Millisecond}
	c := New(Options{BaseURL: s.url, HTTPClient: hc})
	n := 0
	for o, err := range c.StreamOrder(ctx(), "q_1") {
		if err != nil || o.Status != "filled" {
			t.Fatal(o, err)
		}
		n++
	}
	if n != 1 || hc.Timeout != 30*time.Millisecond || s.rec.header.Get("x-api-key") != "" {
		t.Fatalf("n=%d timeout=%v", n, hc.Timeout)
	}
}

func TestStreamErrors(t *testing.T) {
	c, rec := newServer(t, 404, `{"error":{"code":"ORDER_NOT_FOUND","message":"No order","requestId":"r1"}}`)
	n := 0
	for o, err := range c.StreamOrder(ctx(), "q_x") {
		n++
		var ae *APIError
		if o != nil || !errors.As(err, &ae) || ae.Code != "ORDER_NOT_FOUND" || ae.Status != 404 || ae.RequestID != "r1" {
			t.Fatalf("got %v %v", o, err)
		}
	}
	if n != 1 || rec.header.Get("accept") != "text/event-stream" {
		t.Fatalf("n=%d header=%v", n, rec.header)
	}

	c, _ = newServer(t, 401, `{"error":{"code":"UNAUTHORIZED","message":"Missing key"}}`)
	for _, err := range c.StreamWebhookEvents(ctx()) {
		var ae *APIError
		if !errors.As(err, &ae) || ae.Status != 401 || ae.Code != "UNAUTHORIZED" {
			t.Fatalf("got %v", err)
		}
	}

	c, _ = newServer(t, 200, "event: order\ndata: {not json\n\n")
	for _, err := range c.StreamOrder(ctx(), "q_1") {
		if err == nil || !strings.Contains(err.Error(), "decode order event") {
			t.Fatalf("got %v", err)
		}
	}
}

func TestTriggerWebhookEvent(t *testing.T) {
	c, rec := newServer(t, 202, `{"eventId":"evt_1","deliveries":2,"payload":`+eventJSON+`}`)
	r, err := c.TriggerWebhookEvent(ctx(), EventOrderFilled)
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/webhooks/trigger" || rec.body != `{"type":"order.filled"}` || rec.header.Get("x-api-key") != "k1" {
		t.Fatalf("req: %+v", rec)
	}
	if r.EventID != "evt_1" || r.Deliveries != 2 || r.Payload.Type != EventOrderFilled || !r.Payload.Sample || string(r.Payload.Raw) != eventJSON {
		t.Fatalf("resp: %+v", r)
	}

	c, _ = newServer(t, 403, `{"error":{"code":"FORBIDDEN","message":"Use a test key","requestId":"r3"}}`)
	_, err = c.TriggerWebhookEvent(ctx(), EventOrderFilled)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" || ae.Status != 403 || ae.RequestID != "r3" {
		t.Fatalf("got %v", err)
	}
}
