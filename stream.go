package layerhop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
)

// WebhookEventType is the type of a webhook event.
type WebhookEventType = string

// Webhook event types. Samples from the dashboard's "Send test" button have type "webhook.test".
const (
	EventOrderSubmitted               WebhookEventType = "order.submitted"
	EventOrderFilled                  WebhookEventType = "order.filled"
	EventOrderRefunded                WebhookEventType = "order.refunded"
	EventOrderFailed                  WebhookEventType = "order.failed"
	EventCheckoutSessionProcessing    WebhookEventType = "checkout.session.processing"
	EventCheckoutSessionCompleted     WebhookEventType = "checkout.session.completed"
	EventCheckoutSessionPaymentFailed WebhookEventType = "checkout.session.payment_failed"
	EventCheckoutSessionExpired       WebhookEventType = "checkout.session.expired"
	EventCheckoutSessionRefunded      WebhookEventType = "checkout.session.refunded"
	EventInvoiceSent                  WebhookEventType = "invoice.sent"
	EventInvoicePaid                  WebhookEventType = "invoice.paid"
	EventSubscriptionCreated          WebhookEventType = "subscription.created"
	EventSubscriptionRenewed          WebhookEventType = "subscription.renewed"
	EventSubscriptionPastDue          WebhookEventType = "subscription.past_due"
	EventSubscriptionCanceled         WebhookEventType = "subscription.canceled"
	EventLimitOrderOpened             WebhookEventType = "limit_order.opened"
	EventLimitOrderFilled             WebhookEventType = "limit_order.filled"
	EventLimitOrderCancelled          WebhookEventType = "limit_order.cancelled"
	EventLimitOrderExpired            WebhookEventType = "limit_order.expired"
	EventLimitOrderFailed             WebhookEventType = "limit_order.failed"
)

// WebhookEvent is the JSON body LayerHop POSTs to your webhook endpoints.
type WebhookEvent struct {
	ID       string           `json:"id"`
	Type     WebhookEventType `json:"type"`
	Livemode bool             `json:"livemode"`
	// Sample is true for samples from the dashboard or TriggerWebhookEvent.
	Sample    bool             `json:"sample,omitempty"`
	CreatedAt string           `json:"createdAt"`
	Data      WebhookEventData `json:"data"`
	// Raw is the exact JSON payload, e.g. to forward it to a local endpoint unchanged.
	Raw json.RawMessage `json:"-"`
}

// WebhookEventData holds the objects an event is about: Order for order.* events, Session for
// checkout.session.* events (plus Refund for checkout.session.refunded), Invoice for invoice.*
// events (plus the paying Session for invoice.paid), Subscription for subscription.* events (plus
// Invoice for subscription.renewed and subscription.past_due), LimitOrder for limit_order.* events
// (plus the fill's Order in Fill for limit_order.filled, sent for every fill, partial ones included).
type WebhookEventData struct {
	Order        *Order           `json:"order,omitempty"`
	Session      *CheckoutSession `json:"session,omitempty"`
	Refund       *Refund          `json:"refund,omitempty"`
	Invoice      *Invoice         `json:"invoice,omitempty"`
	Subscription *Subscription    `json:"subscription,omitempty"`
	LimitOrder   *LimitOrder      `json:"limitOrder,omitempty"`
	Fill         *Order           `json:"fill,omitempty"`
}

// UnmarshalJSON decodes the event and keeps a copy of the payload in Raw.
func (e *WebhookEvent) UnmarshalJSON(b []byte) error {
	type plain WebhookEvent
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*e = WebhookEvent(p)
	e.Raw = append(json.RawMessage(nil), b...)
	return nil
}

// TriggerWebhookResponse is the body of POST /v1/webhooks/trigger.
type TriggerWebhookResponse struct {
	EventID string `json:"eventId"`
	// Deliveries is the number of test endpoints the event was queued for.
	Deliveries int          `json:"deliveries"`
	Payload    WebhookEvent `json:"payload"`
}

// StreamOrder streams an order's status (GET /v1/orders/{quoteId}/events, Server-Sent Events). It
// yields the order now and on every change, and ends when the order is final (filled, refunded,
// failed or expired) and the server closes the stream. On an error (an *APIError such as
// ORDER_NOT_FOUND, a network error or ctx.Err()) it yields (nil, err) once and stops. Breaking out
// of the loop or cancelling ctx closes the connection.
//
//	for o, err := range client.StreamOrder(ctx, quoteID) {
//		if err != nil { return err }
//		fmt.Println(o.Status)
//	}
func (c *Client) StreamOrder(ctx context.Context, quoteID string) iter.Seq2[*Order, error] {
	return streamSSE[Order](ctx, c, "/orders/"+url.PathEscape(quoteID)+"/events", "order")
}

// StreamCheckoutSession streams a checkout session (GET /v1/checkout/sessions/{id}/events). Public.
// It yields the session now and on every change until it is paid, expired or needs_review. Errors
// and cancellation work as in StreamOrder.
func (c *Client) StreamCheckoutSession(ctx context.Context, id string) iter.Seq2[*CheckoutSession, error] {
	return streamSSE[CheckoutSession](ctx, c, "/checkout/sessions/"+url.PathEscape(id)+"/events", "session")
}

// StreamWebhookEvents streams your account's webhook events of the API key's mode (live or test) as
// they are created, with the exact payloads your endpoints receive (GET /v1/webhooks/stream). Needs
// an API key. The server closes the stream after at most 30 minutes; reconnect to continue.
func (c *Client) StreamWebhookEvents(ctx context.Context) iter.Seq2[*WebhookEvent, error] {
	return streamSSE[WebhookEvent](ctx, c, "/webhooks/stream", "event")
}

// TriggerWebhookEvent creates a sample event of eventType in test mode (POST /v1/webhooks/trigger).
// It is delivered to your test endpoints that subscribe to it and to open StreamWebhookEvents
// streams. Test keys only: a live key returns a FORBIDDEN *APIError.
func (c *Client) TriggerWebhookEvent(ctx context.Context, eventType WebhookEventType) (*TriggerWebhookResponse, error) {
	var out TriggerWebhookResponse
	if err := c.do(ctx, http.MethodPost, "/webhooks/trigger", nil, map[string]string{"type": eventType}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// streamHTTP is the HTTP client for long-lived streams: the configured client without its overall
// Timeout (which would cut every stream), so only ctx ends them.
func (c *Client) streamHTTP() *http.Client {
	if c.http.Timeout == 0 {
		return c.http
	}
	hc := *c.http
	hc.Timeout = 0
	return &hc
}

// streamSSE reads a Server-Sent Events stream, yielding the JSON data of `event` frames.
func streamSSE[T any](ctx context.Context, c *Client, path, event string) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1"+path, nil)
		if err != nil {
			yield(nil, err)
			return
		}
		req.Header.Set("accept", "text/event-stream")
		req.Header.Set("user-agent", c.opts.UserAgent)
		if c.opts.APIKey != "" {
			req.Header.Set("x-api-key", c.opts.APIKey)
		}
		res, err := c.streamHTTP().Do(req)
		if err != nil {
			yield(nil, err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			yield(nil, decodeAPIError(res, raw))
			return
		}
		br := bufio.NewReader(res.Body)
		name, data := "message", []string(nil)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				// A clean close ends the stream; a partial trailing frame is dropped.
				if err != io.EOF {
					if ctx.Err() != nil {
						err = ctx.Err()
					}
					yield(nil, err)
				}
				return
			}
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			switch {
			case line == "":
				if name == event && len(data) > 0 {
					v := new(T)
					if err := json.Unmarshal([]byte(strings.Join(data, "\n")), v); err != nil {
						yield(nil, fmt.Errorf("layerhop: decode %s event: %w", event, err))
						return
					}
					if !yield(v, nil) {
						return
					}
				}
				name, data = "message", nil
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimPrefix(line[len("data:"):], " "))
			}
		}
	}
}
