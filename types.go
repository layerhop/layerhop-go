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

// ProviderID is a liquidity provider id ("relay", "debridge", "jupiter", ...). "sandbox" serves
// every request made with an lh_test_ key; "direct" is a plain token transfer, used by checkout
// when the payer already holds the settlement token.
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
	// Livemode is false when the request used an lh_test_ key (sandbox quotes). Nil means live.
	Livemode *bool `json:"livemode,omitempty"`
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

	// Sandbox is true in test mode: the tx is a placeholder that moves no funds. Never broadcast it.
	Sandbox bool `json:"sandbox,omitempty"`
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
	// Livemode is false for test-mode orders. Nil means live.
	Livemode *bool `json:"livemode,omitempty"`
	// CheckoutSessionID is the checkout session this order pays, if any.
	CheckoutSessionID string `json:"checkoutSessionId,omitempty"`
	// DepositAddressID is the deposit address this order came from, if any.
	DepositAddressID string `json:"depositAddressId,omitempty"`
	// RefundID is the checkout refund this order executes, if any.
	RefundID  string `json:"refundId,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
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

// CheckoutStatus is the status of a checkout session.
type CheckoutStatus = string

// Checkout session statuses. Paid is final; treat Processing as not yet paid.
const (
	CheckoutOpen        CheckoutStatus = "open"
	CheckoutProcessing  CheckoutStatus = "processing"
	CheckoutPaid        CheckoutStatus = "paid"
	CheckoutExpired     CheckoutStatus = "expired"
	CheckoutNeedsReview CheckoutStatus = "needs_review"
)

// CreateCheckoutParams are the body of POST /v1/checkout/sessions. Empty optional fields are omitted.
type CreateCheckoutParams struct {
	// Amount due, an integer string in base units of the settlement token (49 USDC = "49000000").
	Amount string `json:"amount"`
	// SettleChain is a chain id or slug: ethereum, arbitrum, base, optimism, bsc, polygon or solana.
	SettleChain string `json:"settleChain"`
	// SettleToken is "USDC" or "USDT" (or its address) on SettleChain.
	SettleToken string `json:"settleToken"`
	// Recipient is your payout address on the settlement chain.
	Recipient    string `json:"recipient"`
	MerchantName string `json:"merchantName,omitempty"`
	// Description is shown to the payer.
	Description string `json:"description,omitempty"`
	// Reference is your own order id; never shown to the payer.
	Reference string `json:"reference,omitempty"`
	// Metadata holds up to 20 string values; never shown to the payer.
	Metadata   map[string]string `json:"metadata,omitempty"`
	SuccessURL string            `json:"successUrl,omitempty"`
	CancelURL  string            `json:"cancelUrl,omitempty"`
	// ExpiresInMinutes is 5-10080; the API default (60) applies when zero.
	ExpiresInMinutes int `json:"expiresInMinutes,omitempty"`
}

// CheckoutSettlement is the token a checkout session is paid out in.
type CheckoutSettlement struct {
	ChainID  int64  `json:"chainId"`
	Token    string `json:"token"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// CheckoutPayment is the current (or final) payment attempt of a session. JSON nulls decode to
// zero values.
type CheckoutPayment struct {
	QuoteID    string `json:"quoteId"`
	Payer      string `json:"payer,omitempty"`
	SrcChainID int64  `json:"srcChainId,omitempty"`
	SrcToken   string `json:"srcToken,omitempty"`
	// SrcAmount is what the payer sent, in base units of SrcToken.
	SrcAmount string `json:"srcAmount,omitempty"`
	SrcTxHash string `json:"srcTxHash,omitempty"`
	// DstTxHash is the verified delivery transaction once paid.
	DstTxHash string `json:"dstTxHash,omitempty"`
	PaidAt    string `json:"paidAt,omitempty"`
}

// CheckoutSession is a payment request ("checkout.session"). Nullable strings decode to "".
type CheckoutSession struct {
	ID           string             `json:"id"`
	Object       string             `json:"object"`
	Livemode     bool               `json:"livemode"`
	Status       CheckoutStatus     `json:"status"`
	StatusDetail string             `json:"statusDetail,omitempty"`
	Amount       string             `json:"amount"`
	Settlement   CheckoutSettlement `json:"settlement"`
	Recipient    string             `json:"recipient"`
	MerchantName string             `json:"merchantName,omitempty"`
	Description  string             `json:"description,omitempty"`
	// URL is the hosted checkout page: send the payer here.
	URL string `json:"url"`
	// ProductID is the product the session was started from, if any.
	ProductID string `json:"productId,omitempty"`
	// InvoiceID is the invoice the session pays, if any.
	InvoiceID string `json:"invoiceId,omitempty"`
	// RefundedAmount is the total of completed refunds, in settlement-token base units ("0" if none).
	RefundedAmount string `json:"refundedAmount,omitempty"`
	SuccessURL     string `json:"successUrl,omitempty"`
	CancelURL      string `json:"cancelUrl,omitempty"`
	ExpiresAt      string `json:"expiresAt"`
	// Payment is nil until a payment is reported.
	Payment *CheckoutPayment `json:"payment"`
	// Reference and Metadata are returned only with the owning account's API key.
	Reference string            `json:"reference,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// ListCheckoutSessionsParams filter GET /v1/checkout/sessions. Empty fields are omitted.
type ListCheckoutSessionsParams struct {
	Status CheckoutStatus
	Cursor string
	// Limit is 1-100 (API default 50).
	Limit int
}

// ListCheckoutSessionsResponse is the body of GET /v1/checkout/sessions.
type ListCheckoutSessionsResponse struct {
	Sessions   []CheckoutSession `json:"sessions"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// CheckoutQuoteParams are the body of POST /v1/checkout/sessions/{id}/quote.
type CheckoutQuoteParams struct {
	// SrcChain is the chain the payer pays from (id or slug).
	SrcChain string `json:"srcChain"`
	// SrcToken is a token address / mint, curated symbol or native symbol.
	SrcToken string `json:"srcToken"`
	// Sender is the payer's address on SrcChain.
	Sender string `json:"sender"`
	// SlippageBps is omitted when zero (API default 50).
	SlippageBps int `json:"slippageBps,omitempty"`
}

// CheckoutQuote is a Quote for paying a session. DstAmountMin is always >= the amount due.
type CheckoutQuote struct {
	Quote
	// SrcAmount is what the payer sends, in base units of the chosen token.
	SrcAmount string `json:"srcAmount"`
}

// CheckoutQuoteResponse is the body of POST /v1/checkout/sessions/{id}/quote. Best == Quotes[0].
type CheckoutQuoteResponse struct {
	SessionID string            `json:"sessionId"`
	Livemode  bool              `json:"livemode"`
	AmountDue string            `json:"amountDue"`
	Best      CheckoutQuote     `json:"best"`
	Quotes    []CheckoutQuote   `json:"quotes"`
	Errors    []ProviderFailure `json:"errors"`
}

// CreateCheckoutProductParams are the body of POST /v1/checkout/products. Empty optional fields
// are omitted.
type CreateCheckoutProductParams struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	MerchantName string `json:"merchantName,omitempty"`
	// Amount is the price, an integer string in base units of the settlement token.
	Amount string `json:"amount"`
	// SettleChain is a chain id or slug: ethereum, arbitrum, base, optimism, bsc, polygon or solana.
	SettleChain string `json:"settleChain"`
	// SettleToken is "USDC" or "USDT" (or its address) on SettleChain.
	SettleToken string `json:"settleToken"`
	// Recipient is your payout address on the settlement chain.
	Recipient  string `json:"recipient"`
	SuccessURL string `json:"successUrl,omitempty"`
	CancelURL  string `json:"cancelUrl,omitempty"`
	// ExpiresInMinutes is the lifetime of each session started from the product (5-10080); the API
	// default (60) applies when zero.
	ExpiresInMinutes int `json:"expiresInMinutes,omitempty"`
	// AllowedOrigins are https origins allowed to start sessions from a browser; empty = any.
	AllowedOrigins []string `json:"allowedOrigins,omitempty"`
	// Active defaults to true when nil.
	Active *bool `json:"active,omitempty"`
}

// UpdateCheckoutProductParams are the body of PATCH /v1/checkout/products/{id}: only non-nil
// fields are sent. A pointer to "" clears a nullable field (Description, MerchantName, SuccessURL,
// CancelURL) by sending null; a non-nil empty AllowedOrigins sends [] (any origin).
type UpdateCheckoutProductParams struct {
	Name             *string
	Description      *string
	MerchantName     *string
	Amount           *string
	SettleChain      *string
	SettleToken      *string
	Recipient        *string
	SuccessURL       *string
	CancelURL        *string
	ExpiresInMinutes *int
	AllowedOrigins   []string
	Active           *bool
}

// MarshalJSON sends only the fields that are set.
func (p UpdateCheckoutProductParams) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	set := func(key string, v *string) {
		if v != nil {
			m[key] = *v
		}
	}
	nullable := func(key string, v *string) {
		if v == nil {
			return
		}
		if *v == "" {
			m[key] = nil
		} else {
			m[key] = *v
		}
	}
	set("name", p.Name)
	nullable("description", p.Description)
	nullable("merchantName", p.MerchantName)
	set("amount", p.Amount)
	set("settleChain", p.SettleChain)
	set("settleToken", p.SettleToken)
	set("recipient", p.Recipient)
	nullable("successUrl", p.SuccessURL)
	nullable("cancelUrl", p.CancelURL)
	if p.ExpiresInMinutes != nil {
		m["expiresInMinutes"] = *p.ExpiresInMinutes
	}
	if p.AllowedOrigins != nil {
		m["allowedOrigins"] = p.AllowedOrigins
	}
	if p.Active != nil {
		m["active"] = *p.Active
	}
	return json.Marshal(m)
}

// CheckoutProduct is a fixed price for payment buttons, layerhop.com/buy/{id} links and store
// plugins ("checkout.product"). Nullable strings decode to "". Recipient, SuccessURL, CancelURL,
// ExpiresInMinutes, AllowedOrigins, CreatedAt and UpdatedAt are returned only with the owner's key.
type CheckoutProduct struct {
	ID           string             `json:"id"`
	Object       string             `json:"object"`
	Livemode     bool               `json:"livemode"`
	Active       bool               `json:"active"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	MerchantName string             `json:"merchantName,omitempty"`
	Amount       string             `json:"amount"`
	Settlement   CheckoutSettlement `json:"settlement"`
	// URL is https://layerhop.com/buy/{id}: it starts a session and opens the payment page.
	URL              string   `json:"url"`
	Recipient        string   `json:"recipient,omitempty"`
	SuccessURL       string   `json:"successUrl,omitempty"`
	CancelURL        string   `json:"cancelUrl,omitempty"`
	ExpiresInMinutes int      `json:"expiresInMinutes,omitempty"`
	AllowedOrigins   []string `json:"allowedOrigins,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
	UpdatedAt        string   `json:"updatedAt,omitempty"`
}

// ListCheckoutProductsParams filter GET /v1/checkout/products. Empty fields are omitted.
type ListCheckoutProductsParams struct {
	// Active filters by state when non-nil.
	Active *bool
	Cursor string
	// Limit is 1-100 (API default 50).
	Limit int
}

// ListCheckoutProductsResponse is the body of GET /v1/checkout/products.
type ListCheckoutProductsResponse struct {
	Products   []CheckoutProduct `json:"products"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// StartProductCheckoutParams are the body of POST /v1/checkout/products/{id}/sessions: the only
// things a caller may add to the product's terms.
type StartProductCheckoutParams struct {
	// Reference is your order id, returned in the session and its webhooks.
	Reference string `json:"reference,omitempty"`
	// Metadata holds up to 10 string values.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// InvoiceStatus is an invoice's lifecycle state.
type InvoiceStatus = string

// Invoice statuses.
const (
	// InvoiceDraft is editable and not visible to the customer.
	InvoiceDraft InvoiceStatus = "draft"
	// InvoiceOpen was sent and can be paid.
	InvoiceOpen InvoiceStatus = "open"
	// InvoicePaid was paid through checkout.
	InvoicePaid InvoiceStatus = "paid"
	// InvoiceVoid was cancelled and can no longer be paid.
	InvoiceVoid InvoiceStatus = "void"
)

// InvoiceLineItem is one line of an invoice. UnitAmount is in base units of the settlement token;
// the line total is UnitAmount × Quantity.
type InvoiceLineItem struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitAmount  string `json:"unitAmount"`
}

// CreateInvoiceParams are the body of POST /v1/invoices. Empty optional fields are omitted.
type CreateInvoiceParams struct {
	// CustomerEmail receives the invoice email (and the receipt once paid).
	CustomerEmail string `json:"customerEmail,omitempty"`
	CustomerName  string `json:"customerName,omitempty"`
	MerchantName  string `json:"merchantName,omitempty"`
	Memo          string `json:"memo,omitempty"`
	// LineItems: 1 to 50 lines; the invoice amount is their sum.
	LineItems []InvoiceLineItem `json:"lineItems"`
	// SettleChain is a chain slug or id ("base", "8453").
	SettleChain string `json:"settleChain"`
	// SettleToken is a stablecoin symbol or address on SettleChain ("USDC").
	SettleToken string `json:"settleToken"`
	Recipient   string `json:"recipient"`
	// DueAt is an ISO 8601 date ("2026-11-01") or date-time.
	DueAt string `json:"dueAt,omitempty"`
	// Send finalizes the invoice and emails it right away (status "open").
	Send bool `json:"send,omitempty"`
}

// UpdateInvoiceParams are the body of PATCH /v1/invoices/{id} (draft invoices only): only non-nil
// fields are sent. A pointer to "" clears a nullable field (CustomerEmail, CustomerName,
// MerchantName, Memo, DueAt) by sending null; a non-nil LineItems replaces every line.
type UpdateInvoiceParams struct {
	CustomerEmail *string
	CustomerName  *string
	MerchantName  *string
	Memo          *string
	LineItems     []InvoiceLineItem
	SettleChain   *string
	SettleToken   *string
	Recipient     *string
	DueAt         *string
}

// MarshalJSON sends only the fields that are set.
func (p UpdateInvoiceParams) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	set := func(key string, v *string) {
		if v != nil {
			m[key] = *v
		}
	}
	nullable := func(key string, v *string) {
		if v == nil {
			return
		}
		if *v == "" {
			m[key] = nil
		} else {
			m[key] = *v
		}
	}
	nullable("customerEmail", p.CustomerEmail)
	nullable("customerName", p.CustomerName)
	nullable("merchantName", p.MerchantName)
	nullable("memo", p.Memo)
	if p.LineItems != nil {
		m["lineItems"] = p.LineItems
	}
	set("settleChain", p.SettleChain)
	set("settleToken", p.SettleToken)
	set("recipient", p.Recipient)
	nullable("dueAt", p.DueAt)
	return json.Marshal(m)
}

// Invoice is a bill sent to a customer by email and paid through checkout ("invoice"). Nullable
// strings decode to "". CustomerEmail, Recipient, SessionID, SentAt, ReceiptSentAt, VoidedAt and
// UpdatedAt are returned only to the owner (empty from GetPublicInvoice).
type Invoice struct {
	ID       string        `json:"id"`
	Object   string        `json:"object"`
	Livemode bool          `json:"livemode"`
	Number   string        `json:"number"`
	Status   InvoiceStatus `json:"status"`
	// MerchantName, CustomerName and Memo are shown on the hosted page and the email.
	MerchantName string             `json:"merchantName,omitempty"`
	CustomerName string             `json:"customerName,omitempty"`
	Memo         string             `json:"memo,omitempty"`
	LineItems    []InvoiceLineItem  `json:"lineItems"`
	Amount       string             `json:"amount"`
	Settlement   CheckoutSettlement `json:"settlement"`
	DueAt        string             `json:"dueAt,omitempty"`
	PaidAt       string             `json:"paidAt,omitempty"`
	// URL is the hosted invoice page (layerhop.com/invoice/{id}).
	URL string `json:"url"`
	// ReceiptURL is the PDF receipt, set once the invoice is paid.
	ReceiptURL    string `json:"receiptUrl,omitempty"`
	CreatedAt     string `json:"createdAt"`
	CustomerEmail string `json:"customerEmail,omitempty"`
	Recipient     string `json:"recipient,omitempty"`
	// SessionID is the checkout session that pays (or last tried to pay) the invoice.
	SessionID     string `json:"sessionId,omitempty"`
	SentAt        string `json:"sentAt,omitempty"`
	ReceiptSentAt string `json:"receiptSentAt,omitempty"`
	VoidedAt      string `json:"voidedAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

// ListInvoicesParams filter GET /v1/invoices. Empty fields are omitted.
type ListInvoicesParams struct {
	Status InvoiceStatus
	Cursor string
	// Limit is 1-100 (server default 20).
	Limit int
}

// ListInvoicesResponse is one page of invoices, newest first.
type ListInvoicesResponse struct {
	Invoices   []Invoice `json:"invoices"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

// PayInvoiceResponse is the checkout session that pays an invoice: send the payer to URL.
type PayInvoiceResponse struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

// DepositAddressStatus is a deposit address's state.
type DepositAddressStatus = string

// Deposit address statuses.
const (
	// DepositAddressActive is tracked: deposits sent to it become orders.
	DepositAddressActive DepositAddressStatus = "active"
	// DepositAddressDisabled is no longer tracked; tell users not to send to it.
	DepositAddressDisabled DepositAddressStatus = "disabled"
)

// CreateDepositAddressParams are the body of POST /v1/deposit-addresses. Empty optional fields are
// omitted.
type CreateDepositAddressParams struct {
	// SrcChain is the chain id or slug the user sends on ("base", "8453").
	SrcChain string `json:"srcChain"`
	// SrcToken is the token the user sends (symbol or address on SrcChain).
	SrcToken string `json:"srcToken"`
	DstChain string `json:"dstChain"`
	DstToken string `json:"dstToken"`
	// Recipient receives DstToken on DstChain.
	Recipient string `json:"recipient"`
	// RefundTo is the user's address on SrcChain for refunds. Never a CEX deposit address.
	RefundTo string `json:"refundTo"`
	// Reusable defaults to true when nil (any amount, forever, for this route). False makes a
	// one-time address for exactly Amount.
	Reusable *bool `json:"reusable,omitempty"`
	// Amount in base units of SrcToken: required when Reusable is false; otherwise it sizes the
	// estimate (API default: about $100 of SrcToken).
	Amount string `json:"amount,omitempty"`
	// Label is your own tag (1-64 chars), filterable in ListDepositAddresses.
	Label string `json:"label,omitempty"`
	// Metadata holds up to 20 string values.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Integrator and IntegratorFeeBps default to the client's Options when empty / nil.
	Integrator       string `json:"integrator,omitempty"`
	IntegratorFeeBps *int   `json:"integratorFeeBps,omitempty"`
}

// DepositAddressFees are the fees applied to every deposit. Integrator is "" without a partner.
type DepositAddressFees struct {
	OurFeeBps        int    `json:"ourFeeBps"`
	IntegratorFeeBps int    `json:"integratorFeeBps"`
	Integrator       string `json:"integrator,omitempty"`
}

// DepositAddressEstimate prices one deposit of Amount (returned by CreateDepositAddress only).
type DepositAddressEstimate struct {
	Amount           string `json:"amount"`
	DstAmount        string `json:"dstAmount"`
	DstAmountMin     string `json:"dstAmountMin"`
	EstimatedSeconds int    `json:"estimatedSeconds"`
}

// DepositAddress is an address the end user sends Token to on ChainID; the provider delivers
// DstToken to Recipient on DstChainID ("deposit_address"). LayerHop never holds the funds. Nullable
// strings decode to "".
type DepositAddress struct {
	ID       string               `json:"id"`
	Object   string               `json:"object"`
	Livemode bool                 `json:"livemode"`
	Status   DepositAddressStatus `json:"status"`
	// Provider is "relay" (live) or "sandbox" (test mode).
	Provider ProviderID `json:"provider"`
	// Address is what your user sends Token to on ChainID.
	Address    string `json:"address"`
	ChainID    int64  `json:"chainId"`
	Token      string `json:"token"`
	DstChainID int64  `json:"dstChainId"`
	DstToken   string `json:"dstToken"`
	Recipient  string `json:"recipient"`
	RefundTo   string `json:"refundTo"`
	Reusable   bool   `json:"reusable"`
	// Amount is set on one-time addresses only (base units of Token).
	Amount        string             `json:"amount,omitempty"`
	Fees          DepositAddressFees `json:"fees"`
	Label         string             `json:"label,omitempty"`
	Metadata      map[string]string  `json:"metadata,omitempty"`
	DepositCount  int                `json:"depositCount"`
	LastDepositAt string             `json:"lastDepositAt,omitempty"`
	CreatedAt     string             `json:"createdAt"`
	// Estimate is set by CreateDepositAddress only.
	Estimate *DepositAddressEstimate `json:"estimate,omitempty"`
	// Deposits are the 20 most recent deposit orders, newest first (GetDepositAddress only).
	Deposits []Order `json:"deposits,omitempty"`
}

// ListDepositAddressesParams filter GET /v1/deposit-addresses. Empty fields are omitted.
type ListDepositAddressesParams struct {
	Status DepositAddressStatus
	Label  string
	Cursor string
	// Limit is 1-100.
	Limit int
}

// ListDepositAddressesResponse is one page of deposit addresses, newest first.
type ListDepositAddressesResponse struct {
	DepositAddresses []DepositAddress `json:"depositAddresses"`
	NextCursor       string           `json:"nextCursor,omitempty"`
}

// SimulateDepositResponse is the body of POST /v1/deposit-addresses/{id}/simulate: the test
// deposit order, settled like any test-mode order.
type SimulateDepositResponse struct {
	Order Order `json:"order"`
}

// RefundStatus is a refund's lifecycle state.
type RefundStatus = string

// Refund statuses.
const (
	// RefundPending: created; waiting for the merchant to sign and report the transaction.
	RefundPending RefundStatus = "pending"
	// RefundSubmitted: the transaction was reported and is being tracked.
	RefundSubmitted RefundStatus = "submitted"
	// RefundCompleted: delivered to the payer; counted in the session's RefundedAmount.
	RefundCompleted RefundStatus = "completed"
	// RefundFailed: the refund order failed or expired (StatusDetail says why).
	RefundFailed RefundStatus = "failed"
)

// Refund destinations for CreateRefundParams.To.
const (
	// RefundToOriginal sends the refund back in the payer's original chain and token (default).
	RefundToOriginal = "original"
	// RefundToSettlement sends the settlement token to the payer's address on the settlement chain.
	RefundToSettlement = "settlement"
)

// CreateRefundParams are the body of POST /v1/checkout/sessions/{id}/refunds. Empty fields are
// omitted.
type CreateRefundParams struct {
	// Amount in settlement-token base units; default: everything not yet refunded.
	Amount string `json:"amount,omitempty"`
	Reason string `json:"reason,omitempty"`
	// From is the wallet that signs the refund (default: the session's payout address).
	From string `json:"from,omitempty"`
	// To is RefundToOriginal (default) or RefundToSettlement.
	To string `json:"to,omitempty"`
}

// RefundDestination is where a refund is delivered.
type RefundDestination struct {
	ChainID int64  `json:"chainId"`
	Token   string `json:"token"`
	Address string `json:"address"`
}

// Refund returns (part of) a paid checkout session to its payer ("refund"). Nullable strings
// decode to "".
type Refund struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"`
	SessionID    string            `json:"sessionId,omitempty"`
	Livemode     bool              `json:"livemode"`
	Status       RefundStatus      `json:"status"`
	StatusDetail string            `json:"statusDetail,omitempty"`
	Amount       string            `json:"amount"`
	Reason       string            `json:"reason,omitempty"`
	QuoteID      string            `json:"quoteId,omitempty"`
	To           RefundDestination `json:"to"`
	TxHash       string            `json:"txHash,omitempty"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt,omitempty"`
}

// RefundQuote is the route that executes a refund. Build it with BuildTx(QuoteID), sign it from
// From, then ReportSubmitted(QuoteID, txHash).
type RefundQuote struct {
	QuoteID          string     `json:"quoteId"`
	Provider         ProviderID `json:"provider"`
	SrcAmount        string     `json:"srcAmount"`
	DstAmount        string     `json:"dstAmount"`
	DstAmountMin     string     `json:"dstAmountMin"`
	EstimatedSeconds int        `json:"estimatedSeconds"`
	ExpiresAt        int64      `json:"expiresAt"`
	// From is the wallet that must sign the refund transaction.
	From string `json:"from"`
}

// CreateRefundResponse is the body of POST /v1/checkout/sessions/{id}/refunds.
type CreateRefundResponse struct {
	Refund Refund      `json:"refund"`
	Quote  RefundQuote `json:"quote"`
	// Next describes the remaining steps (build, sign, report).
	Next string `json:"next"`
}

// ListRefundsResponse is every refund of a session, newest first.
type ListRefundsResponse struct {
	Refunds []Refund `json:"refunds"`
}

// StatusDay is one day of a component's uptime history. Uptime is a percentage, nil without data.
type StatusDay struct {
	Date   string   `json:"date"`
	Uptime *float64 `json:"uptime"`
}

// StatusComponent is one monitored component. Status is "operational", "degraded" or "down".
type StatusComponent struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Group     string   `json:"group"`
	Status    string   `json:"status"`
	LatencyMs *int     `json:"latencyMs"`
	Uptime90d *float64 `json:"uptime90d"`
	// Days are the last 90 days, oldest first.
	Days []StatusDay `json:"days"`
}

// StatusIncident is an incident from the last 14 days. ResolvedAt is "" while it is ongoing.
type StatusIncident struct {
	ID          string `json:"id"`
	ComponentID string `json:"componentId"`
	Component   string `json:"component"`
	Title       string `json:"title"`
	StartedAt   string `json:"startedAt"`
	ResolvedAt  string `json:"resolvedAt,omitempty"`
}

// StatusResponse is the body of GET /v1/status (status.layerhop.com). Status is "operational",
// "degraded" or "outage".
type StatusResponse struct {
	Status     string            `json:"status"`
	UpdatedAt  string            `json:"updatedAt"`
	Components []StatusComponent `json:"components"`
	Incidents  []StatusIncident  `json:"incidents"`
	URL        string            `json:"url"`
}
