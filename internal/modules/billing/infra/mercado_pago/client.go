package mercado_pago

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"oficina-tech/internal/modules/billing/domain/payment"
)

const defaultBaseURL = "https://api.mercadopago.com"

type Client struct {
	baseURL         string
	accessToken     string
	notificationURL string
	httpClient      *http.Client
}

type Option func(*Client)

func NewClientFromEnv(options ...Option) *Client {
	baseURL := defaultBaseURL
	if override := strings.TrimSpace(os.Getenv("MP_BASE_URL")); override != "" {
		baseURL = strings.TrimRight(override, "/")
	}
	client := &Client{
		baseURL:         baseURL,
		accessToken:     strings.TrimSpace(os.Getenv("MP_ACCESS_TOKEN")),
		notificationURL: strings.TrimSpace(os.Getenv("MP_NOTIFICATION_URL")),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, option := range options {
		option(client)
	}
	return client
}

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithAccessToken(accessToken string) Option {
	return func(c *Client) {
		c.accessToken = accessToken
	}
}

func WithNotificationURL(notificationURL string) Option {
	return func(c *Client) {
		c.notificationURL = notificationURL
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func (c *Client) CreatePreference(ctx context.Context, orderID string, items []payment.PreferenceItem, externalRef string) (*payment.Preference, error) {
	if c.accessToken == "" {
		return nil, payment.ErrMissingAccessToken
	}
	if externalRef == "" {
		externalRef = orderID
	}

	requestItems := make([]preferenceItemRequest, 0, len(items))
	for _, item := range items {
		requestItems = append(requestItems, preferenceItemRequest{
			Title:     item.Title,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
		})
	}

	reqBody := preferenceRequest{
		Items:             requestItems,
		ExternalReference: externalRef,
		NotificationURL:   c.notificationURL,
	}

	var response preferenceResponse
	if err := c.doJSON(ctx, http.MethodPost, "/checkout/preferences", reqBody, &response); err != nil {
		return nil, err
	}

	initURL := response.InitPoint
	if initURL == "" {
		initURL = response.SandboxInitPoint
	}
	return &payment.Preference{
		ID:      response.ID,
		InitURL: initURL,
	}, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID string) (*payment.Payment, error) {
	if c.accessToken == "" {
		return nil, payment.ErrMissingAccessToken
	}

	var response paymentResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/payments/"+paymentID, nil, &response); err != nil {
		return nil, err
	}

	return &payment.Payment{
		ID:                fmt.Sprint(response.ID),
		Status:            response.Status,
		ExternalReference: response.ExternalReference,
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				continue
			}
			return err
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode >= 500 && attempt < 2 {
			lastErr = fmt.Errorf("mercado pago returned %d: %s", resp.StatusCode, string(respBody))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("mercado pago returned %d: %s", resp.StatusCode, string(respBody))
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
		return nil
	}
	return lastErr
}
