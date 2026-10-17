package layerhop

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// DefaultWebhookTolerance is the maximum age of a webhook signature timestamp.
const DefaultWebhookTolerance = 300 * time.Second

// Webhook verification errors.
var (
	ErrWebhookMalformedHeader = errors.New("layerhop: malformed LayerHop-Signature header")
	ErrWebhookTimestamp       = errors.New("layerhop: webhook timestamp outside tolerance")
	ErrWebhookSignature       = errors.New("layerhop: webhook signature mismatch")
)

// VerifyWebhookSignature checks a `LayerHop-Signature: t=<unix>,v1=<hex>` header against the raw
// request body. The signature is hex(HMAC-SHA256(secret, "<t>.<body>")); several v1 values are
// accepted (secret rotation). tolerance <= 0 means DefaultWebhookTolerance; a zero now means
// time.Now(). Pass the body exactly as received, before any JSON parsing.
func VerifyWebhookSignature(secret string, body []byte, header string, tolerance time.Duration, now time.Time) error {
	if tolerance <= 0 {
		tolerance = DefaultWebhookTolerance
	}
	if now.IsZero() {
		now = time.Now()
	}
	var ts string
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			if b, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, b)
			}
		}
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrWebhookMalformedHeader
	}
	age := now.Sub(time.Unix(t, 0))
	if age < 0 {
		age = -age
	}
	if age > tolerance {
		return ErrWebhookTimestamp
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, s := range sigs {
		if hmac.Equal(s, want) {
			return nil
		}
	}
	return ErrWebhookSignature
}

// SignWebhook builds a LayerHop-Signature header value. Useful for testing your webhook handler.
func SignWebhook(secret string, body []byte, t time.Time) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
