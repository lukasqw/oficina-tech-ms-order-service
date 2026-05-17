package http_clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"oficina-tech/internal/shared/infra/http/middleware"
)

const (
	defaultTimeout = 5 * time.Second
	maxAttempts    = 3
)


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
	url := c.baseURL + path

	ctx, span := otel.Tracer("oficina-tech/http-client").Start(ctx, "HTTP GET "+path,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPRequestMethodKey.String(http.MethodGet),
			semconv.URLFull(url),
			semconv.ServerAddress(c.baseURL),
		),
	)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if authorization, ok := ctx.Value(middleware.AuthorizationKey).(string); ok && authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	// Injeta traceparent/tracestate no header para que o MS de destino
	// continue o mesmo trace (W3C Trace Context propagation).
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(semconv.HTTPResponseStatusCode(resp.StatusCode))

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		_, _ = io.Copy(io.Discard, resp.Body)
		span.SetStatus(codes.Error, "not found")
		return ErrNotFound
	case http.StatusUnauthorized:
		_, _ = io.Copy(io.Discard, resp.Body)
		span.SetStatus(codes.Error, "unauthorized")
		return ErrUnauthorized
	default:
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("upstream %d", resp.StatusCode))
			return retryableStatusError{status: resp.StatusCode}
		}
		span.SetStatus(codes.Error, fmt.Sprintf("upstream %d", resp.StatusCode))
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
