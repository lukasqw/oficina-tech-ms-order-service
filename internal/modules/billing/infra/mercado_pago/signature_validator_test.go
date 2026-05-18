package mercado_pago

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"oficina-tech/internal/modules/billing/domain/payment"
)

func TestSignatureValidatorValidSignature(t *testing.T) {
	secret := "secret"
	paymentID := "123456"
	requestID := "bb56a2f1-6aae-46ac-982e-9dcd3581d08e"
	ts := "1742505638683"
	signature := sign(secret, "id:"+paymentID+";request-id:"+requestID+";ts:"+ts+";")

	validator := NewSignatureValidator(secret)
	if err := validator.Validate("ts="+ts+",v1="+signature, requestID, paymentID); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSignatureValidatorInvalidSignature(t *testing.T) {
	validator := NewSignatureValidator("secret")
	err := validator.Validate("ts=1742505638683,v1=bad", "request-id", "123456")
	if err != payment.ErrInvalidWebhookSignature {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func TestSignatureValidatorMalformedHeader(t *testing.T) {
	validator := NewSignatureValidator("secret")
	err := validator.Validate("v1=abc", "request-id", "123456")
	if err != payment.ErrInvalidWebhookSignature {
		t.Fatalf("expected malformed signature error, got %v", err)
	}
}

func TestSignatureValidatorValidSignatureWithoutRequestID(t *testing.T) {
	secret := "secret"
	paymentID := "123456"
	ts := "1742505638683"
	signature := sign(secret, "id:"+paymentID+";ts:"+ts+";")

	validator := NewSignatureValidator(secret)
	if err := validator.Validate("ts="+ts+",v1="+signature, "", paymentID); err != nil {
		t.Fatalf("Validate() without x-request-id error = %v", err)
	}
}

func TestSignatureValidatorMissingSecret(t *testing.T) {
	validator := NewSignatureValidator("")
	err := validator.Validate("ts=1,v1=abc", "request-id", "123456")
	if err != payment.ErrMissingWebhookSecret {
		t.Fatalf("expected missing secret error, got %v", err)
	}
}

func sign(secret, manifest string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	return hex.EncodeToString(mac.Sum(nil))
}
