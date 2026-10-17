package layerhop_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	layerhop "github.com/layerhop/layerhop-go"
)

// A fake API so the example runs offline.
func fakeAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/quote":
			io.WriteString(w, `{"best":{"quoteId":"q_1","provider":"mock","dstAmount":"612345678","dstAmountMin":"609283949","estimatedSeconds":8,"fees":{"ourFeeBps":20},"expiresAt":1760000030},"quotes":[],"errors":[]}`)
		case r.URL.Path == "/v1/orders":
			io.WriteString(w, `{"quoteId":"q_1","provider":"mock","tx":{"vm":"evm","chainId":8453,"to":"0xto","data":"0x","value":"0"}}`)
		case strings.HasSuffix(r.URL.Path, "/submitted"):
			io.WriteString(w, `{"quoteId":"q_1","status":"submitted"}`)
		default:
			io.WriteString(w, `{"provider":"mock","id":"0xhash","state":"filled"}`)
		}
	}))
}

func Example() {
	srv := fakeAPI()
	defer srv.Close()
	ctx := context.Background()

	lh := layerhop.New(layerhop.Options{BaseURL: srv.URL}) // omit BaseURL for https://api.layerhop.com

	// 1. Quote 100 USDC on Base -> SOL on Solana. Amounts are base-unit strings.
	q, err := lh.Quote(ctx, layerhop.QuoteParams{
		SrcChain: "base", SrcToken: "USDC", Amount: "100000000",
		DstChain: "solana", DstToken: "SOL",
		Sender: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", Recipient: "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("best:", q.Best.Provider, q.Best.DstAmount)

	// 2. Build the unsigned transaction.
	tx, err := lh.BuildTx(ctx, q.Best.QuoteID)
	if err != nil {
		panic(err)
	}
	fmt.Println("vm:", tx.VM, "chain:", tx.ChainID)

	// 3. Sign and broadcast with your own wallet library (go-ethereum, solana-go, ...).
	txHash := "0xhash"

	// 4. Tell LayerHop, then 5. watch until final.
	if _, err := lh.ReportSubmitted(ctx, q.Best.QuoteID, txHash); err != nil {
		panic(err)
	}
	for st, err := range lh.Watch(ctx, q.Best.Provider, txHash, layerhop.WatchOptions{Interval: time.Millisecond}) {
		if err != nil {
			panic(err)
		}
		fmt.Println("state:", st.State)
	}
	// Output:
	// best: mock 612345678
	// vm: evm chain: 8453
	// state: filled
}

func ExampleVerifyWebhookSignature() {
	body := []byte(`{"id":"evt_1","type":"order.filled"}`)
	header := layerhop.SignWebhook("whsec_123", body, time.Now()) // what LayerHop sends in LayerHop-Signature
	err := layerhop.VerifyWebhookSignature("whsec_123", body, header, 0, time.Time{})
	fmt.Println(err == nil)
	// Output: true
}
