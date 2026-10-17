package layerhop

import (
	"bytes"
	"encoding/json"
)

// NumString is an integer kept as a decimal string. It decodes from a JSON string or number,
// so big values (gas, base-unit amounts) never lose precision.
type NumString string

// UnmarshalJSON accepts "123", 123 or null.
func (n *NumString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*n = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*n = NumString(s)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	*n = NumString(num.String())
	return nil
}

// ProviderID is a liquidity provider id ("relay", "debridge", "jupiter", ...).
type ProviderID = string

// OrderState is the state reported by GET /v1/orders/{provider}/{id}.
type OrderState = string

// Order states. Filled, Refunded and Failed are final.
const (
	StatePending  OrderState = "pending"
	StateFilled   OrderState = "filled"
	StateRefunded OrderState = "refunded"
	StateFailed   OrderState = "failed"
	StateUnknown  OrderState = "unknown"
)

// IsFinal reports whether the state will not change any more.
func IsFinal(s OrderState) bool {
	return s == StateFilled || s == StateRefunded || s == StateFailed
}

// SolanaChainID is LayerHop's internal chain id for Solana.
const SolanaChainID = 7565164

// Health is the body of GET /v1/health.
type Health struct {
	OK        bool         `json:"ok"`
	Providers []ProviderID `json:"providers"`
}

// Chain is one entry of the chain registry.
type Chain struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	VM             string `json:"vm"`
	NativeSymbol   string `json:"nativeSymbol"`
	NativeDecimals int    `json:"nativeDecimals"`
	ExplorerTx     string `json:"explorerTx"`
}

// Token is one entry of the built-in token list.
type Token struct {
	ChainID  int64  `json:"chainId"`
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// QuoteParams are the inputs of GET /v1/quote.
type QuoteParams struct {
	// SrcChain is a chain id or slug ("base", "solana", "8453").
	SrcChain string
	// SrcToken is a symbol from the token list ("USDC") or an address / mint.
	SrcToken string
	// Amount is an integer string in base units of SrcToken.
	Amount    string
	DstChain  string
	DstToken  string
	Sender    string
	Recipient string
	// SlippageBps defaults to 50 when zero.
	SlippageBps int
}

// FeeBreakdown describes the fees applied to a quote.
type FeeBreakdown struct {
	OurFeeBps        int      `json:"ourFeeBps"`
	ProviderFeeUsd   *float64 `json:"providerFeeUsd,omitempty"`
	GasUsd           *float64 `json:"gasUsd,omitempty"`
	IntegratorFeeBps *int     `json:"integratorFeeBps,omitempty"`
	Integrator       string   `json:"integrator,omitempty"`
}

// Quote is one provider's priced route.
type Quote struct {
	QuoteID          string       `json:"quoteId"`
	Provider         ProviderID   `json:"provider"`
	DstAmount        string       `json:"dstAmount"`
	DstAmountMin     string       `json:"dstAmountMin"`
	EstimatedSeconds int          `json:"estimatedSeconds"`
	Fees             FeeBreakdown `json:"fees"`
	// ExpiresAt is in unix seconds.
	ExpiresAt int64 `json:"expiresAt"`
}

// ProviderFailure is a provider that failed while the request as a whole succeeded.
type ProviderFailure struct {
	Provider string `json:"provider"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// QuoteResponse is the body of GET /v1/quote. Best == Quotes[0].
type QuoteResponse struct {
	Best   Quote             `json:"best"`
	Quotes []Quote           `json:"quotes"`
	Errors []ProviderFailure `json:"errors"`
}

// Approval is an ERC-20 approve(spender, amount) that must be mined before the main tx.
type Approval struct {
	Token   string `json:"token"`
	Spender string `json:"spender"`
	Amount  string `json:"amount"`
}

// BuiltTx is the transaction for the user's wallet, discriminated by VM ("evm" | "svm").
type BuiltTx struct {
	VM string `json:"vm"`

	// EVM fields.
	ChainID  int64     `json:"chainId,omitempty"`
	To       string    `json:"to,omitempty"`
	Data     string    `json:"data,omitempty"`
	Value    string    `json:"value,omitempty"`
	Approval *Approval `json:"approval,omitempty"`

	// SVM fields.
	// SerializedTx is a base64 serialized VersionedTransaction.
	SerializedTx string `json:"serializedTx,omitempty"`
	// SubmitViaAPI: sign only, then send through Client.Submit instead of an RPC node.
	SubmitViaAPI bool `json:"submitViaApi,omitempty"`
}

// IsEVM reports whether this is an EVM transaction.
func (t BuiltTx) IsEVM() bool { return t.VM == "evm" }

// IsSVM reports whether this is a Solana transaction.
func (t BuiltTx) IsSVM() bool { return t.VM == "svm" }

// ReportSubmittedResponse is the body of POST /v1/orders/{quoteId}/submitted.
type ReportSubmittedResponse struct {
	QuoteID string `json:"quoteId"`
	Status  string `json:"status"`
}

// SimulationError explains why a simulation failed.
type SimulationError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Logs    []string `json:"logs,omitempty"`
}

// SimulateResponse is the body of POST /v1/simulate.
type SimulateResponse struct {
	QuoteID  string     `json:"quoteId,omitempty"`
	Provider ProviderID `json:"provider,omitempty"`
	OK       bool       `json:"ok"`
	VM       string     `json:"vm"`
	// Simulated is "tx" (the swap itself) or "approval" (only the approval could be simulated).
	Simulated string `json:"simulated,omitempty"`
	// GasEstimate (EVM) as a decimal string; accepts a JSON number or string.
	GasEstimate NumString `json:"gasEstimate,omitempty"`
	// ComputeUnits (Solana) as a decimal string; accepts a JSON number or string.
	ComputeUnits  NumString        `json:"computeUnits,omitempty"`
	NeedsApproval bool             `json:"needsApproval,omitempty"`
	Error         *SimulationError `json:"error,omitempty"`
	Tx            *BuiltTx         `json:"tx,omitempty"`
}

// OrderStatus is the body of GET /v1/orders/{provider}/{id}.
type OrderStatus struct {
	Provider  ProviderID `json:"provider"`
	ID        string     `json:"id"`
	State     OrderState `json:"state"`
	SrcTxHash string     `json:"srcTxHash,omitempty"`
	DstTxHash string     `json:"dstTxHash,omitempty"`
}

// Order is one row of GET /v1/orders. Fields are optional because the listing is lenient.
type Order struct {
	QuoteID       string     `json:"quoteId,omitempty"`
	Provider      ProviderID `json:"provider,omitempty"`
	Status        string     `json:"status,omitempty"`
	StatusDetail  string     `json:"statusDetail,omitempty"`
	Channel       string     `json:"channel,omitempty"`
	OurFeeBps     int        `json:"ourFeeBps,omitempty"`
	PartnerFeeBps int        `json:"partnerFeeBps,omitempty"`
	SrcChainID    int64      `json:"srcChainId,omitempty"`
	SrcToken      string     `json:"srcToken,omitempty"`
	DstChainID    int64      `json:"dstChainId,omitempty"`
	DstToken      string     `json:"dstToken,omitempty"`
	Amount        string     `json:"amount,omitempty"`
	DstAmount     string     `json:"dstAmount,omitempty"`
	Sender        string     `json:"sender,omitempty"`
	Recipient     string     `json:"recipient,omitempty"`
	SrcTxHash     string     `json:"srcTxHash,omitempty"`
	DstTxHash     string     `json:"dstTxHash,omitempty"`
	CreatedAt     string     `json:"createdAt,omitempty"`
	UpdatedAt     string     `json:"updatedAt,omitempty"`
}

// ListOrdersParams filter GET /v1/orders. Empty fields are omitted.
type ListOrdersParams struct {
	// Address matches the sender or the recipient.
	Address string
	Status  string
	Cursor  string
	Limit   int
}

// ListOrdersResponse is the body of GET /v1/orders.
type ListOrdersResponse struct {
	Orders     []Order `json:"orders"`
	NextCursor string  `json:"nextCursor,omitempty"`
}
