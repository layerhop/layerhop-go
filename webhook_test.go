package layerhop

import (
	"errors"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"id":"evt_1","type":"order.filled","data":{}}`)
	now := time.Unix(1760000000, 0)
	hdr := SignWebhook(secret, body, now)

	// Known vector: hex(HMAC-SHA256("whsec_test", "1760000000.<body>")) is checked for shape here.
	if len(hdr) != len("t=1760000000,v1=")+64 {
		t.Fatalf("header: %s", hdr)
	}

	cases := []struct {
		name   string
		secret string
		body   []byte
		header string
		now    time.Time
		want   error
	}{
		{"valid", secret, body, hdr, now, nil},
		{"valid within tolerance", secret, body, hdr, now.Add(299 * time.Second), nil},
		{"valid with rotated extra v1", secret, body, "t=1760000000,v1=" + "00" + hdr[18:] + ",v1=" + hdr[16:], now, nil},
		{"spaces tolerated", secret, body, "t=1760000000, v1=" + hdr[16:], now, nil},
		{"too old", secret, body, hdr, now.Add(301 * time.Second), ErrWebhookTimestamp},
		{"from the future", secret, body, hdr, now.Add(-301 * time.Second), ErrWebhookTimestamp},
		{"wrong secret", "other", body, hdr, now, ErrWebhookSignature},
		{"tampered body", secret, []byte(`{"id":"evt_2"}`), hdr, now, ErrWebhookSignature},
		{"missing t", secret, body, "v1=" + hdr[16:], now, ErrWebhookMalformedHeader},
		{"missing v1", secret, body, "t=1760000000", now, ErrWebhookMalformedHeader},
		{"garbage", secret, body, "nonsense", now, ErrWebhookMalformedHeader},
	}
	for _, c := range cases {
		err := VerifyWebhookSignature(c.secret, c.body, c.header, 0, c.now)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}

	if err := VerifyWebhookSignature(secret, body, hdr, 10*time.Second, now.Add(11*time.Second)); !errors.Is(err, ErrWebhookTimestamp) {
		t.Errorf("custom tolerance: %v", err)
	}
}

func TestVerifyWebhookKnownVector(t *testing.T) {
	// echo -n '1700000000.{"a":1}' | openssl dgst -sha256 -hmac secret
	const want = "t=1700000000,v1=" + knownVector
	got := SignWebhook("secret", []byte(`{"a":1}`), time.Unix(1700000000, 0))
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

const knownVector = "49f24e537407743fa4a0242bb63b94b9a47ee99cbbe071ccd8a22550ae411686"
