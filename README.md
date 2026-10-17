# layerhop-go

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
| `ListOrders(ctx, ListOrdersParams)` | `GET /v1/orders?address=&status=&cursor=&limit=` |

`Options` has these fields: `BaseURL` (default `https://api.layerhop.com`), `APIKey` (sent as `x-api-key`), `Channel` (default `api`), `Integrator`, `IntegratorFeeBps`, `HTTPClient` and `UserAgent`.

### Watch

`Watch` returns an `iter.Seq2[OrderStatus, error]`. It yields once per state change. It stops after a final state or after an error. The error can be an `*APIError`, `ctx.Err()` or `ErrWatchTimeout`, which is raised after 30 minutes by default. Breaking out of the loop stops the polling.

### Errors

Any non-2xx response returns an `*APIError` with these fields:
- `Code`: a stable code, such as `QUOTE_EXPIRED`. See https://docs.layerhop.com/errors.
- `Message`
- `Status`
- `RequestID`

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

## Development

```bash
cd sdks/go && go vet ./... && go test ./...
```

## License

MIT
