package http_clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"oficina-tech/internal/shared/infra/http/middleware"
)

const (
	defaultTimeout = 5 * time.Second
	maxAttempts    = 3
)

type envelope[T any] struct {
	Data   T             `json:"data"`
	Errors []errorDetail `json:"errors"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type restClient struct {
	baseURL string
	client  *http.Client
}

func newRESTClient(baseURL string) restClient {
	return restClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

func (c restClient) get(ctx context.Context, path string, output any) error {
	var lastErr error
	backoffs := []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := c.doGet(ctx, path, output)
		if err == nil {
			return nil
		}
		lastErr = err
		if !shouldRetry(err) || attempt == maxAttempts-1 {
			return err
		}
		timer := time.NewTimer(backoffs[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return lastErr
}

type retryableStatusError struct {
	status int
}

func (e retryableStatusError) Error() string {
	return fmt.Sprintf("upstream returned retryable status %d", e.status)
}

func shouldRetry(err error) bool {
	_, ok := err.(retryableStatusError)
	return ok
}

func (c restClient) doGet(ctx context.Context, path string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if authorization, ok := ctx.Value(middleware.AuthorizationKey).(string); ok && authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		io.Copy(io.Discard, resp.Body)
		return ErrNotFound
	case http.StatusUnauthorized:
		io.Copy(io.Discard, resp.Body)
		return ErrUnauthorized
	default:
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 500 {
			return retryableStatusError{status: resp.StatusCode}
		}
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	wrapped := struct {
		Data   json.RawMessage `json:"data"`
		Errors []errorDetail   `json:"errors"`
	}{}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Data != nil {
		return json.Unmarshal(wrapped.Data, output)
	}

	return json.Unmarshal(raw, output)
}
