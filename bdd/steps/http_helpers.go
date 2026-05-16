package steps

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// envelope mirrors the standard response wrapper used by the three services:
//
//	{ "data": {...}, "errors": [...] }
type envelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []envelopeError `json:"errors"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func doJSON(ctx context.Context, w *World, method, url string, body any, token string) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func decodeData(raw []byte, target any) error {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("api errors: %+v", env.Errors)
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("api response has no data")
	}
	return json.Unmarshal(env.Data, target)
}

// expectStatus returns an error if the actual HTTP status differs from the
// expected one. The body is included in the error to keep failure messages
// actionable when running against a live stack.
func expectStatus(expected, got int, body []byte) error {
	if expected == got {
		return nil
	}
	return fmt.Errorf("expected HTTP %d, got %d, body: %s", expected, got, string(body))
}

// adminToken authenticates with the bootstrapped admin user once per test run
// (the JWT TTL is well above the suite duration) and returns the bearer token.
// MS1 /auth/login expects CPF + password — see access_control auth handler.
func adminToken(ctx context.Context, w *World) (string, error) {
	adminTokenOnce.Do(func() {
		body := map[string]string{
			"cpf":      defaultEnv("ADMIN_CPF", "11122233344"),
			"password": w.AdminPass,
		}
		status, raw, err := doJSON(ctx, w, http.MethodPost, w.MS1URL+"/auth/login", body, "")
		if err != nil {
			cachedAdminErr = err
			return
		}
		if status != http.StatusOK {
			cachedAdminErr = fmt.Errorf("admin login: %d %s", status, string(raw))
			return
		}
		var resp struct {
			Token string `json:"token"`
		}
		if err := decodeData(raw, &resp); err != nil {
			cachedAdminErr = err
			return
		}
		cachedAdmin = resp.Token
	})
	return cachedAdmin, cachedAdminErr
}

// signMercadoPagoWebhook returns the headers Mercado Pago would set on a
// payment webhook POST so MS2 accepts it as authentic.
func signMercadoPagoWebhook(secret, paymentID string) (signature, requestID string) {
	requestID = "req-" + paymentID
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	manifest := "id:" + paymentID + ";request-id:" + requestID + ";ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	signature = "ts=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	return signature, requestID
}
