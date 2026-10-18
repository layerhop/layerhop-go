# layerhop-go

[![Go Reference](https://pkg.go.dev/badge/github.com/layerhop/layerhop-go.svg)](https://pkg.go.dev/github.com/layerhop/layerhop-go) [![license](https://img.shields.io/badge/license-MIT-7ddec8?labelColor=090d15)](https://opensource.org/license/mit) [![docs](https://img.shields.io/badge/docs-docs.layerhop.com-7ddec8?labelColor=090d15)](https://docs.layerhop.com/sdks/go)

Go client for the [LayerHop](https://layerhop.com) API, which gives you best-price cross-chain and same-chain swaps through one non-custodial API. It mirrors the TypeScript `@layerhop/sdk`.

- Uses only the standard library. Needs Go 1.23+, because `Watch` returns an `iter.Seq2`.
- Every method takes a `context.Context`.
- Amounts are always **integer strings in base units** (`"100000000"` = 100 USDC). They are never floats.
- **The SDK never holds private keys.** It returns unsigned transactions. Your own wallet library signs and broadcasts them.

## Install

```bash
go get github.com/layerhop/layerhop-go
```

## Quickstart

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	layerhop "github.com/layerhop/layerhop-go"
)

func main() {
	ctx := context.Background()
	lh := layerhop.New(layerhop.Options{APIKey: os.Getenv("LAYERHOP_API_KEY")})

	// 1. Quote: 100 USDC on Base -> SOL on Solana.
	q, err := lh.Quote(ctx, layerhop.QuoteParams{
		SrcChain: "base", SrcToken: "USDC", Amount: "100000000",
		DstChain: "solana", DstToken: "SOL",
		Sender:    "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		Recipient: "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
	})
	var apiErr *layerhop.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Code, apiErr.DocsURL()) // e.g. NO_ROUTE
		return
	} else if err != nil {
		panic(err)
	}

	// 2. Build the unsigned transaction (call this before q.Best.ExpiresAt).
	tx, err := lh.BuildTx(ctx, q.Best.QuoteID)
	if err != nil {
		panic(err)
	}

	// 3. Sign and broadcast with your own wallet library.
	//    EVM (go-ethereum): if tx.Approval != nil, send ERC-20 approve(spender, amount) first,
	//    then send {To: tx.To, Data: tx.Data, Value: tx.Value} on chain tx.ChainID.
	//    Solana (solana-go): decode tx.SerializedTx (base64) and sign it. If tx.SubmitViaAPI is true,
	//    call lh.Submit(ctx, quoteID, signedBase64) instead of sending it to an RPC node.
	txHash := signAndSend(tx)

	// 4. Report the broadcast so LayerHop tracks the order (and fires webhooks).
	if _, err := lh.ReportSubmitted(ctx, q.Best.QuoteID, txHash); err != nil {
		panic(err)
	}

	// 5. Watch until filled / refunded / failed.
	for st, err := range lh.Watch(ctx, q.Best.Provider, txHash, layerhop.WatchOptions{}) {
		if err != nil {
			panic(err)
		}
		fmt.Println(st.State, st.DstTxHash)
	}
}
```

## API

| Method | Endpoint |
|---|---|
| `Health(ctx)` | `GET /v1/health` |
| `Chains(ctx)` | `GET /v1/chains` |
| `Tokens(ctx, chain)` | `GET /v1/tokens?chain=` |
| `Quote(ctx, QuoteParams)` | `GET /v1/quote` |
| `BuildTx(ctx, quoteID)` | `POST /v1/orders` |
| `Submit(ctx, quoteID, signedTxBase64)` | `POST /v1/orders/submit` (only for Solana `SubmitViaAPI` txs) |
| `ReportSubmitted(ctx, quoteID, txHash)` | `POST /v1/orders/{quoteId}/submitted` |
| `Simulate(ctx, quoteID)` | `POST /v1/simulate` |
| `Status(ctx, provider, id)` | `GET /v1/orders/{provider}/{id}` |
| `Watch(ctx, provider, id, WatchOptions)` | polls `Status` and yields each state change until the order is final |
| `StreamOrder(ctx, quoteID)` | `GET /v1/orders/{quoteId}/events` (SSE), yields the order until final |
| `ListOrders(ctx, ListOrdersParams)` | `GET /v1/orders?address=&status=&cursor=&limit=` |
| `CreateCheckoutSession(ctx, CreateCheckoutParams)` | `POST /v1/checkout/sessions` |
| `ListCheckoutSessions(ctx, ListCheckoutSessionsParams)` | `GET /v1/checkout/sessions?status=&cursor=&limit=` |
| `GetCheckoutSession(ctx, id)` | `GET /v1/checkout/sessions/{id}` (public) |
| `ExpireCheckoutSession(ctx, id)` | `POST /v1/checkout/sessions/{id}/expire` |
| `QuoteCheckoutSession(ctx, id, CheckoutQuoteParams)` | `POST /v1/checkout/sessions/{id}/quote` (public) |
| `StreamCheckoutSession(ctx, id)` | `GET /v1/checkout/sessions/{id}/events` (SSE, public) |
| `CreateCheckoutProduct(ctx, CreateCheckoutProductParams)` | `POST /v1/checkout/products` |
| `ListCheckoutProducts(ctx, ListCheckoutProductsParams)` | `GET /v1/checkout/products?active=&cursor=&limit=` |
| `GetCheckoutProduct(ctx, id)` | `GET /v1/checkout/products/{id}` (public) |
| `UpdateCheckoutProduct(ctx, id, UpdateCheckoutProductParams)` | `PATCH /v1/checkout/products/{id}` (only set fields) |
| `StartProductCheckout(ctx, id, StartProductCheckoutParams)` | `POST /v1/checkout/products/{id}/sessions` (public) |
| `StreamWebhookEvents(ctx)` | `GET /v1/webhooks/stream` (SSE, API key) |
| `TriggerWebhookEvent(ctx, eventType)` | `POST /v1/webhooks/trigger` (test keys only) |

`Options` has these fields: `BaseURL` (default `https://api.layerhop.com`), `APIKey` (sent as `x-api-key`), `Channel` (default `api`), `Integrator`, `IntegratorFeeBps`, `HTTPClient` and `UserAgent`.

### Watch

`Watch` returns an `iter.Seq2[OrderStatus, error]`. It yields once per state change. It stops after a final state or after an error. The error can be an `*APIError`, `ctx.Err()` or `ErrWatchTimeout`, which is raised after 30 minutes by default. Breaking out of the loop stops the polling.

### Live order status

`StreamOrder` follows an order over Server-Sent Events instead of polling. It returns an `iter.Seq2[*Order, error]` that yields the order now and on every change, and ends once the order is final (`filled`, `refunded`, `failed` or `expired`). `StreamCheckoutSession` does the same for a checkout session (until `paid`, `expired` or `needs_review`).

```go
for o, err := range lh.StreamOrder(ctx, q.Best.QuoteID) {
	if err != nil {
		return err // *APIError (ORDER_NOT_FOUND), a network error or ctx.Err()
	}
	fmt.Println(o.Status, o.DstTxHash)
}
```

An error is yielded once and ends the loop. Breaking out of the loop or cancelling `ctx` closes the connection. Streams are long-lived, so they ignore `HTTPClient.Timeout`; bound them with `ctx`. See the [live status guide](https://docs.layerhop.com/guides/live-status).

### Errors

Any non-2xx response returns an `*APIError` with these fields:
- `Code`: a stable code, such as `QUOTE_EXPIRED`. See https://docs.layerhop.com/errors.
- `Message`
- `Status`
- `RequestID`

## Checkout (pay with any token)

Get paid in USDC or USDT on the chain you choose while your customer pays with any token on any chain. Create a session with your API key and send the customer to its hosted `URL`. The session becomes `paid` (and a `checkout.session.completed` webhook fires) once the delivery is verified on chain.

```go
s, err := lh.CreateCheckoutSession(ctx, layerhop.CreateCheckoutParams{
	Amount: "49000000", // 49 USDC (6 decimals)
	SettleChain: "base", SettleToken: "USDC",
	Recipient:   "0xYourPayoutAddress",
	Description: "Pro plan, 1 month",
	Reference:   "order_1042",
	SuccessURL:  "https://shop.example.com/thanks",
})
if err != nil {
	panic(err)
}
fmt.Println(s.URL) // redirect the customer here

s, _ = lh.GetCheckoutSession(ctx, s.ID) // s.Status: open | processing | paid | expired | needs_review
```

To build your own payment page, `GetCheckoutSession` and `QuoteCheckoutSession` are public (no key needed). Pay the chosen quote with `BuildTx` and `ReportSubmitted` as usual:

```go
q, err := lh.QuoteCheckoutSession(ctx, sessionID, layerhop.CheckoutQuoteParams{
	SrcChain: "arbitrum", SrcToken: "ETH", Sender: payerAddress,
})
// q.Best.SrcAmount is what the payer sends; q.Best.DstAmountMin is at least the amount due.
tx, err := lh.BuildTx(ctx, q.Best.QuoteID)
```

`ListCheckoutSessions` pages through your sessions and `ExpireCheckoutSession` cancels an open one. See the [checkout guide](https://docs.layerhop.com/guides/checkout).

## Products and payment buttons

A product is a fixed price (amount, settlement token, payout address and redirect URLs) that anyone may start a session from without being able to change it: the backing for payment buttons, `https://layerhop.com/buy/{id}` links and store plugins. `CreateCheckoutProduct`, `ListCheckoutProducts` and `UpdateCheckoutProduct` need your API key; `GetCheckoutProduct` and `StartProductCheckout` are public.

```go
p, err := lh.CreateCheckoutProduct(ctx, layerhop.CreateCheckoutProductParams{
	Name:           "Pro plan",
	Description:    "1 month",
	Amount:         "49000000", // 49 USDC
	SettleChain:    "base",
	SettleToken:    "USDC",
	Recipient:      "0xYourPayoutAddress",
	SuccessURL:     "https://shop.example.com/thanks",
	AllowedOrigins: []string{"https://shop.example.com"},
})
fmt.Println(p.URL) // https://layerhop.com/buy/prod_...: share it or put it behind a button

// Only the fields you set change; a pointer to "" clears a nullable field.
off := false
lh.UpdateCheckoutProduct(ctx, p.ID, layerhop.UpdateCheckoutProductParams{Active: &off}) // stops new sessions

// Public (no key): start a session on the product's terms, adding only your order id.
s, err := layerhop.New(layerhop.Options{}).StartProductCheckout(ctx, p.ID, layerhop.StartProductCheckoutParams{Reference: "order_1042"})
fmt.Println(s.URL, s.ProductID)
```

See the [payment buttons guide](https://docs.layerhop.com/guides/payment-buttons).

## Test mode

Use an `lh_test_` key exactly like a live key. Every request is served by the `sandbox` provider, responses carry `livemode: false` (`QuoteResponse.Livemode`, `Order.Livemode`), and built transactions have `Sandbox: true`. Never broadcast them; report any hash with `ReportSubmitted`. The last digits of `Amount` choose the outcome: `…4001` refunds, `…4002` fails and `…4004` returns `NO_ROUTE`. Checkout sessions created with a test key (`cs_test_…`) are paid through the sandbox too. See the [test mode guide](https://docs.layerhop.com/guides/test-mode).

## Webhooks

```go
func handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body) // raw bytes, before JSON parsing
	err := layerhop.VerifyWebhookSignature(secret, body, r.Header.Get("LayerHop-Signature"), 0, time.Time{})
	if err != nil { // ErrWebhookMalformedHeader | ErrWebhookTimestamp | ErrWebhookSignature
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	// ...
}
```

The signature is `t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>`. By default, timestamps older or newer than 300 s are rejected. Use `SignWebhook` to produce test headers.

### Webhook tools

With an API key, `StreamWebhookEvents` yields your account's webhook events (in the key's live or test mode) as they are created. Each `*WebhookEvent` has typed `Data.Order` / `Data.Session` and the exact payload in `Raw`. `TriggerWebhookEvent` creates a sample event in test mode (test keys only; a live key returns a `FORBIDDEN` `*APIError`).

```go
lh := layerhop.New(layerhop.Options{APIKey: "lh_test_..."})
r, err := lh.TriggerWebhookEvent(ctx, layerhop.EventOrderFilled) // r.EventID, r.Deliveries, r.Payload
if err != nil {
	return err
}
for e, err := range lh.StreamWebhookEvents(ctx) {
	if err != nil {
		return err
	}
	fmt.Println(e.Type, string(e.Raw))
}
```

## Development

```bash
go vet ./... && go test ./...
```

## License

MIT
