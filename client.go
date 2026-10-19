// Package layerhop is a Go client for the LayerHop cross-chain swap API.
//
// The client never holds private keys: it returns unsigned transactions that your own wallet
// library signs and broadcasts. Amounts are integer strings in base units.
package layerhop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the hosted API origin.
const DefaultBaseURL = "https://api.layerhop.com"

// Version is the SDK version, sent in the User-Agent header.
const Version = "0.5.0"

// Options configure a Client. The zero value is valid.
type Options struct {
	// BaseURL defaults to https://api.layerhop.com. A trailing slash is removed.
	BaseURL string
	// APIKey is sent as the x-api-key header.
	APIKey string
	// Channel is the fee channel sent with every quote (default "api").
	Channel string
	// Integrator is your partner id, sent as `integrator` on every quote.
	Integrator string
	// IntegratorFeeBps overrides the partner fee; honored only with a partner APIKey.
	IntegratorFeeBps *int
	// HTTPClient defaults to a client with a 30 s timeout.
	HTTPClient *http.Client
	// UserAgent defaults to "layerhop-go/<Version>".
	UserAgent string
}

// Client talks to the LayerHop REST API. It is safe for concurrent use.
type Client struct {
	opts    Options
	baseURL string
	http    *http.Client
}

// New returns a Client.
func New(opts Options) *Client {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if opts.Channel == "" {
		opts.Channel = "api"
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "layerhop-go/" + Version
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{opts: opts, baseURL: base, http: hc}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + "/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("accept", "application/json")
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	req.Header.Set("user-agent", c.opts.UserAgent)
	if c.opts.APIKey != "" {
		req.Header.Set("x-api-key", c.opts.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return decodeAPIError(res, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("layerhop: decode %s %s: %w", method, path, err)
	}
	return nil
}

// Health calls GET /v1/health.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var out Health
	if err := c.do(ctx, http.MethodGet, "/health", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Chains lists every chain LayerHop can route from or to (GET /v1/chains).
func (c *Client) Chains(ctx context.Context) ([]Chain, error) {
	var out struct {
		Chains []Chain `json:"chains"`
	}
	if err := c.do(ctx, http.MethodGet, "/chains", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Chains, nil
}

// Tokens lists the built-in tokens of one chain (id or slug) (GET /v1/tokens).
func (c *Client) Tokens(ctx context.Context, chain string) ([]Token, error) {
	var out struct {
		Tokens []Token `json:"tokens"`
	}
	if err := c.do(ctx, http.MethodGet, "/tokens", url.Values{"chain": {chain}}, nil, &out); err != nil {
		return nil, err
	}
	return out.Tokens, nil
}

// Quote asks every provider for a price (GET /v1/quote). Best is the highest output for the user.
func (c *Client) Quote(ctx context.Context, p QuoteParams) (*QuoteResponse, error) {
	slip := p.SlippageBps
	if slip == 0 {
		slip = 50
	}
	q := url.Values{
		"srcChain":    {p.SrcChain},
		"srcToken":    {p.SrcToken},
		"amount":      {p.Amount},
		"dstChain":    {p.DstChain},
		"dstToken":    {p.DstToken},
		"sender":      {p.Sender},
		"recipient":   {p.Recipient},
		"slippageBps": {strconv.Itoa(slip)},
		"channel":     {c.opts.Channel},
	}
	if c.opts.Integrator != "" {
		q.Set("integrator", c.opts.Integrator)
	}
	if c.opts.IntegratorFeeBps != nil {
		q.Set("integratorFeeBps", strconv.Itoa(*c.opts.IntegratorFeeBps))
	}
	var out QuoteResponse
	if err := c.do(ctx, http.MethodGet, "/quote", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BuildTx returns the transaction to sign for a quote (POST /v1/orders).
// EVM transactions may include an Approval to send first.
func (c *Client) BuildTx(ctx context.Context, quoteID string) (*BuiltTx, error) {
	var out struct {
		Tx BuiltTx `json:"tx"`
	}
	if err := c.do(ctx, http.MethodPost, "/orders", nil, map[string]string{"quoteId": quoteID}, &out); err != nil {
		return nil, err
	}
	return &out.Tx, nil
}

// Submit sends a wallet-signed Solana tx (base64) for SubmitViaAPI quotes (POST /v1/orders/submit).
// It returns the transaction signature.
func (c *Client) Submit(ctx context.Context, quoteID, signedTx string) (string, error) {
	var out struct {
		TxHash string `json:"txHash"`
	}
	body := map[string]string{"quoteId": quoteID, "signedTransaction": signedTx}
	if err := c.do(ctx, http.MethodPost, "/orders/submit", nil, body, &out); err != nil {
		return "", err
	}
	return out.TxHash, nil
}

// ReportSubmitted tells LayerHop the tx for a quote was broadcast, so it can be tracked
// (POST /v1/orders/{quoteId}/submitted). Idempotent.
func (c *Client) ReportSubmitted(ctx context.Context, quoteID, txHash string) (*ReportSubmittedResponse, error) {
	var out ReportSubmittedResponse
	path := "/orders/" + url.PathEscape(quoteID) + "/submitted"
	if err := c.do(ctx, http.MethodPost, path, nil, map[string]string{"txHash": txHash}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Simulate builds the tx for a quote and dry-runs it without recording an order (POST /v1/simulate).
func (c *Client) Simulate(ctx context.Context, quoteID string) (*SimulateResponse, error) {
	var out SimulateResponse
	if err := c.do(ctx, http.MethodPost, "/simulate", nil, map[string]string{"quoteId": quoteID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Status returns the current state of an order (GET /v1/orders/{provider}/{id}).
// id is the provider order id or the source tx hash.
func (c *Client) Status(ctx context.Context, provider ProviderID, id string) (*OrderStatus, error) {
	var out OrderStatus
	path := "/orders/" + url.PathEscape(provider) + "/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOrders lists orders by sender or recipient (GET /v1/orders).
func (c *Client) ListOrders(ctx context.Context, p ListOrdersParams) (*ListOrdersResponse, error) {
	q := url.Values{}
	if p.Address != "" {
		q.Set("address", p.Address)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ListOrdersResponse
	if err := c.do(ctx, http.MethodGet, "/orders", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCheckoutSession creates a checkout session ("pay with any token") and returns it with its
// hosted URL (POST /v1/checkout/sessions). Needs an API key; an lh_test_ key creates a test-mode
// session.
func (c *Client) CreateCheckoutSession(ctx context.Context, p CreateCheckoutParams) (*CheckoutSession, error) {
	var out CheckoutSession
	if err := c.do(ctx, http.MethodPost, "/checkout/sessions", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCheckoutSession returns a checkout session (GET /v1/checkout/sessions/{id}). Public: without
// the owner's API key, Reference and Metadata are empty.
func (c *Client) GetCheckoutSession(ctx context.Context, id string) (*CheckoutSession, error) {
	var out CheckoutSession
	if err := c.do(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCheckoutSessions lists your sessions of the key's mode, newest first (GET /v1/checkout/sessions).
func (c *Client) ListCheckoutSessions(ctx context.Context, p ListCheckoutSessionsParams) (*ListCheckoutSessionsResponse, error) {
	q := url.Values{}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ListCheckoutSessionsResponse
	if err := c.do(ctx, http.MethodGet, "/checkout/sessions", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExpireCheckoutSession cancels an open session so it can no longer be paid
// (POST /v1/checkout/sessions/{id}/expire). Any other status is a CONFLICT APIError.
func (c *Client) ExpireCheckoutSession(ctx context.Context, id string) (*CheckoutSession, error) {
	var out CheckoutSession
	path := "/checkout/sessions/" + url.PathEscape(id) + "/expire"
	if err := c.do(ctx, http.MethodPost, path, nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QuoteCheckoutSession prices paying a session with SrcToken on SrcChain from Sender
// (POST /v1/checkout/sessions/{id}/quote). Public. Pay the chosen quote with BuildTx and
// ReportSubmitted as usual.
func (c *Client) QuoteCheckoutSession(ctx context.Context, id string, p CheckoutQuoteParams) (*CheckoutQuoteResponse, error) {
	var out CheckoutQuoteResponse
	path := "/checkout/sessions/" + url.PathEscape(id) + "/quote"
	if err := c.do(ctx, http.MethodPost, path, nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCheckoutProduct creates a product: a fixed price for payment buttons, layerhop.com/buy/{id}
// links and store plugins (POST /v1/checkout/products). Needs an API key; its mode decides the
// product's mode.
func (c *Client) CreateCheckoutProduct(ctx context.Context, p CreateCheckoutProductParams) (*CheckoutProduct, error) {
	var out CheckoutProduct
	if err := c.do(ctx, http.MethodPost, "/checkout/products", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCheckoutProduct returns a product (GET /v1/checkout/products/{id}). Public: name, price and
// settlement; with the owner's API key, every field.
func (c *Client) GetCheckoutProduct(ctx context.Context, id string) (*CheckoutProduct, error) {
	var out CheckoutProduct
	if err := c.do(ctx, http.MethodGet, "/checkout/products/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCheckoutProducts lists your products of the key's mode (GET /v1/checkout/products).
func (c *Client) ListCheckoutProducts(ctx context.Context, p ListCheckoutProductsParams) (*ListCheckoutProductsResponse, error) {
	q := url.Values{}
	if p.Active != nil {
		q.Set("active", strconv.FormatBool(*p.Active))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ListCheckoutProductsResponse
	if err := c.do(ctx, http.MethodGet, "/checkout/products", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCheckoutProduct changes only the fields that are set (PATCH /v1/checkout/products/{id}).
// Active false stops new sessions; existing sessions keep their terms.
func (c *Client) UpdateCheckoutProduct(ctx context.Context, id string, p UpdateCheckoutProductParams) (*CheckoutProduct, error) {
	var out CheckoutProduct
	if err := c.do(ctx, http.MethodPatch, "/checkout/products/"+url.PathEscape(id), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartProductCheckout starts a checkout session on a product's terms
// (POST /v1/checkout/products/{id}/sessions). Public, no key needed; the caller may only add a
// Reference and Metadata. An inactive product is a CONFLICT APIError. Send the payer to the
// session's URL.
func (c *Client) StartProductCheckout(ctx context.Context, id string, p StartProductCheckoutParams) (*CheckoutSession, error) {
	var out CheckoutSession
	path := "/checkout/products/" + url.PathEscape(id) + "/sessions"
	if err := c.do(ctx, http.MethodPost, path, nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInvoice creates an invoice (POST /v1/invoices). It starts as a draft unless Send is true,
// which finalizes it and emails it to CustomerEmail. Needs an API key; its mode decides the
// invoice's mode.
func (c *Client) CreateInvoice(ctx context.Context, p CreateInvoiceParams) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodPost, "/invoices", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetInvoice returns one of your invoices (GET /v1/invoices/{id}).
func (c *Client) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodGet, "/invoices/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListInvoices lists your invoices of the key's mode, newest first (GET /v1/invoices).
func (c *Client) ListInvoices(ctx context.Context, p ListInvoicesParams) (*ListInvoicesResponse, error) {
	q := url.Values{}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ListInvoicesResponse
	if err := c.do(ctx, http.MethodGet, "/invoices", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateInvoice edits a draft invoice; only the fields that are set change
// (PATCH /v1/invoices/{id}). Any other status is a CONFLICT APIError.
func (c *Client) UpdateInvoice(ctx context.Context, id string, p UpdateInvoiceParams) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodPatch, "/invoices/"+url.PathEscape(id), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendInvoice finalizes a draft (status "open") and emails it to the customer; on an open invoice
// it re-sends the email (POST /v1/invoices/{id}/send). Paid or void is a CONFLICT APIError.
func (c *Client) SendInvoice(ctx context.Context, id string) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodPost, "/invoices/"+url.PathEscape(id)+"/send", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// VoidInvoice cancels a draft or open invoice so it can no longer be paid
// (POST /v1/invoices/{id}/void). Paid or void is a CONFLICT APIError.
func (c *Client) VoidInvoice(ctx context.Context, id string) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodPost, "/invoices/"+url.PathEscape(id)+"/void", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPublicInvoice returns the customer's view of a sent invoice (GET /v1/invoices/{id}/public).
// Public, no key needed; owner-only fields are empty. Drafts are NOT_FOUND.
func (c *Client) GetPublicInvoice(ctx context.Context, id string) (*Invoice, error) {
	var out Invoice
	if err := c.do(ctx, http.MethodGet, "/invoices/"+url.PathEscape(id)+"/public", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PayInvoice starts (or reuses) the checkout session that pays an open invoice
// (POST /v1/invoices/{id}/pay). Public, no key needed. Send the payer to the returned URL.
func (c *Client) PayInvoice(ctx context.Context, id string) (*PayInvoiceResponse, error) {
	var out PayInvoiceResponse
	if err := c.do(ctx, http.MethodPost, "/invoices/"+url.PathEscape(id)+"/pay", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateDepositAddress creates a deposit address (POST /v1/deposit-addresses): your user sends
// SrcToken to the returned Address on SrcChain, without connecting a wallet, and the provider
// delivers DstToken to Recipient. LayerHop never holds the funds; each deposit becomes an order
// (DepositAddressID set) with the usual order.* webhooks. The response carries an Estimate.
// Needs an API key. Live mode may be disabled by the operator: a live key then gets a FORBIDDEN
// APIError (test keys always work). Integrator and IntegratorFeeBps default to the client's Options.
func (c *Client) CreateDepositAddress(ctx context.Context, p CreateDepositAddressParams) (*DepositAddress, error) {
	if p.Integrator == "" {
		p.Integrator = c.opts.Integrator
	}
	if p.IntegratorFeeBps == nil {
		p.IntegratorFeeBps = c.opts.IntegratorFeeBps
	}
	var out DepositAddress
	if err := c.do(ctx, http.MethodPost, "/deposit-addresses", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDepositAddress returns one of your deposit addresses with its 20 most recent deposits in
// Deposits (GET /v1/deposit-addresses/{id}).
func (c *Client) GetDepositAddress(ctx context.Context, id string) (*DepositAddress, error) {
	var out DepositAddress
	if err := c.do(ctx, http.MethodGet, "/deposit-addresses/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListDepositAddresses lists your deposit addresses of the key's mode, newest first
// (GET /v1/deposit-addresses).
func (c *Client) ListDepositAddresses(ctx context.Context, p ListDepositAddressesParams) (*ListDepositAddressesResponse, error) {
	q := url.Values{}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Label != "" {
		q.Set("label", p.Label)
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	var out ListDepositAddressesResponse
	if err := c.do(ctx, http.MethodGet, "/deposit-addresses", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RefreshDepositAddress checks the address for deposits now; call it when your user says they sent
// (POST /v1/deposit-addresses/{id}/refresh). A disabled address is a CONFLICT APIError.
func (c *Client) RefreshDepositAddress(ctx context.Context, id string) (*DepositAddress, error) {
	var out DepositAddress
	if err := c.do(ctx, http.MethodPost, "/deposit-addresses/"+url.PathEscape(id)+"/refresh", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisableDepositAddress stops tracking the address (POST /v1/deposit-addresses/{id}/disable). Tell
// your user not to send to it again.
func (c *Client) DisableDepositAddress(ctx context.Context, id string) (*DepositAddress, error) {
	var out DepositAddress
	if err := c.do(ctx, http.MethodPost, "/deposit-addresses/"+url.PathEscape(id)+"/disable", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SimulateDeposit records a test deposit of amount (base units of the source token) on a test-mode
// address (POST /v1/deposit-addresses/{id}/simulate). Test keys only (FORBIDDEN otherwise). The
// order settles like any test order (amounts ending in 4001 / 4002 end refunded / failed).
func (c *Client) SimulateDeposit(ctx context.Context, id, amount string) (*SimulateDepositResponse, error) {
	var out SimulateDepositResponse
	path := "/deposit-addresses/" + url.PathEscape(id) + "/simulate"
	if err := c.do(ctx, http.MethodPost, path, nil, map[string]string{"amount": amount}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRefund refunds (part of) a paid checkout session (POST /v1/checkout/sessions/{id}/refunds).
// Non-custodial: the response holds a quote; build it with BuildTx, sign it from Quote.From, then
// ReportSubmitted. The refund completes when the order fills (checkout.session.refunded).
func (c *Client) CreateRefund(ctx context.Context, sessionID string, p CreateRefundParams) (*CreateRefundResponse, error) {
	var out CreateRefundResponse
	path := "/checkout/sessions/" + url.PathEscape(sessionID) + "/refunds"
	if err := c.do(ctx, http.MethodPost, path, nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRefunds lists a session's refunds, newest first (GET /v1/checkout/sessions/{id}/refunds).
func (c *Client) ListRefunds(ctx context.Context, sessionID string) (*ListRefundsResponse, error) {
	var out ListRefundsResponse
	path := "/checkout/sessions/" + url.PathEscape(sessionID) + "/refunds"
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ServiceStatus returns LayerHop's public service status, the same data as status.layerhop.com
// (GET /v1/status): overall state, per-component status and 90-day uptime, and recent incidents.
// (Status, by contrast, tracks one order.)
func (c *Client) ServiceStatus(ctx context.Context) (*StatusResponse, error) {
	var out StatusResponse
	if err := c.do(ctx, http.MethodGet, "/status", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WatchOptions tune Watch. Zero values mean Interval 3 s and Timeout 30 min.
type WatchOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

// ErrWatchTimeout is yielded when Watch gives up before the order reached a final state.
var ErrWatchTimeout = fmt.Errorf("layerhop: watch timed out before the order was final")

// Watch polls Status until the order is final (filled / refunded / failed), yielding each state
// change. On an error (including ctx cancellation and ErrWatchTimeout) it yields (zero, err) once
// and stops. Breaking out of the loop stops polling.
//
//	for st, err := range client.Watch(ctx, "relay", txHash, layerhop.WatchOptions{}) {
//		if err != nil { return err }
//		fmt.Println(st.State)
//	}
func (c *Client) Watch(ctx context.Context, provider ProviderID, id string, o WatchOptions) iter.Seq2[OrderStatus, error] {
	if o.Interval <= 0 {
		o.Interval = 3 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Minute
	}
	return func(yield func(OrderStatus, error) bool) {
		deadline := time.Now().Add(o.Timeout)
		last := ""
		for {
			st, err := c.Status(ctx, provider, id)
			if err != nil {
				yield(OrderStatus{}, err)
				return
			}
			if st.State != last {
				last = st.State
				if !yield(*st, nil) {
					return
				}
			}
			if IsFinal(st.State) {
				return
			}
			if !time.Now().Add(o.Interval).Before(deadline) {
				yield(OrderStatus{}, ErrWatchTimeout)
				return
			}
			t := time.NewTimer(o.Interval)
			select {
			case <-ctx.Done():
				t.Stop()
				yield(OrderStatus{}, ctx.Err())
				return
			case <-t.C:
			}
		}
	}
}
