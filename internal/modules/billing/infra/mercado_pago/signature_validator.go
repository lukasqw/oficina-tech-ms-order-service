package mercado_pago

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"oficina-tech/internal/modules/billing/domain/payment"
)

type SignatureValidator struct {
	secret string
}

func NewSignatureValidator(secret string) *SignatureValidator {
	return &SignatureValidator{secret: strings.TrimSpace(secret)}
}

func (v *SignatureValidator) Validate(xSignature, xRequestID, paymentID string) error {
	if v.secret == "" {
		return payment.ErrMissingWebhookSecret
	}
	ts, signature, ok := parseSignatureHeader(xSignature)
	if !ok || paymentID == "" {
		return payment.ErrInvalidWebhookSignature
	}

	// MP docs: se algum campo estiver ausente na notificação, removê-lo do manifesto.
	// data.id deve ser lowercase (IDs de order chegam em maiúsculas: ORD01...).
	manifest := "id:" + strings.ToLower(paymentID) + ";"
	if xRequestID != "" {
		manifest += "request-id:" + xRequestID + ";"
	}
	manifest += "ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(v.secret))
	_, _ = mac.Write([]byte(manifest))
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return payment.ErrInvalidWebhookSignature
	}
	return nil
}

func parseSignatureHeader(header string) (string, string, bool) {
	var ts string
	var v1 string
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "ts":
			ts = strings.TrimSpace(value)
		case "v1":
			v1 = strings.TrimSpace(value)
		}
	}
	return ts, v1, ts != "" && v1 != ""
}
