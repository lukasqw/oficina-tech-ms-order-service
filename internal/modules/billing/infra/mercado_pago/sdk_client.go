package mercado_pago

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/mercadopago/sdk-go/pkg/config"
	sdkpreference "github.com/mercadopago/sdk-go/pkg/preference"
	sdkrefund "github.com/mercadopago/sdk-go/pkg/refund"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// SDKClient implementa payment.MercadoPagoClient usando o SDK oficial (Preferences API).
type SDKClient struct {
	preferenceClient sdkpreference.Client
	refundClient     sdkrefund.Client
	notificationURL  string
	callbackBaseURL  string
	accessToken      string
	apiBaseURL       string // host para chamadas HTTP diretas (get_payment)
	isSandbox        bool   // true when MP_ACCESS_TOKEN starts with "TEST-"
}

// NewSDKClient cria um SDKClient com as credenciais e opções fornecidas.
// Para apontar para o mock BDD, passe config.WithHTTPClient(NewRewritingRequester(baseURL)).
func NewSDKClient(accessToken, notificationURL, callbackBaseURL string, opts ...config.Option) (*SDKClient, error) {
	if accessToken == "" {
		return nil, payment.ErrMissingAccessToken
	}
	cfg, err := config.New(accessToken, opts...)
	if err != nil {
		return nil, fmt.Errorf("mp sdk config: %w", err)
	}
	return &SDKClient{
		preferenceClient: sdkpreference.NewClient(cfg),
		refundClient:     sdkrefund.NewClient(cfg),
		notificationURL:  notificationURL,
		callbackBaseURL:  strings.TrimRight(callbackBaseURL, "/"),
		accessToken:      accessToken,
		apiBaseURL:       "https://api.mercadopago.com",
		isSandbox:        strings.HasPrefix(accessToken, "TEST-"),
	}, nil
}

// NewSDKClientFromEnv inicializa o SDKClient a partir das variáveis de ambiente.
// Se MP_BASE_URL estiver definido, usa o RewritingRequester para redirecionar
// as chamadas ao host alternativo (mock BDD em desenvolvimento).
func NewSDKClientFromEnv() (*SDKClient, error) {
	accessToken := strings.TrimSpace(os.Getenv("MP_ACCESS_TOKEN"))
	notificationURL := strings.TrimSpace(os.Getenv("MP_NOTIFICATION_URL"))
	callbackBaseURL := strings.TrimSpace(os.Getenv("MP_CALLBACK_BASE_URL"))

	opts := []config.Option{}
	baseURL := strings.TrimSpace(os.Getenv("MP_BASE_URL"))
	if baseURL != "" {
		opts = append(opts, config.WithHTTPClient(NewRewritingRequester(baseURL)))
	}

	client, err := NewSDKClient(accessToken, notificationURL, callbackBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	if baseURL != "" {
		client.apiBaseURL = strings.TrimRight(baseURL, "/")
	}
	// MP_SANDBOX=true força modo sandbox independente do prefixo do token.
	// Necessário pois credenciais de teste no MP atual usam prefixo APP_USR-, não TEST-.
	if os.Getenv("MP_SANDBOX") == "true" {
		client.isSandbox = true
	}
	return client, nil
}

// CreateOrder cria uma Preference "Checkout Pro" e retorna o sandbox_init_point/init_point para redirect.
func (c *SDKClient) CreateOrder(ctx context.Context, items []payment.OrderItem, payer payment.PayerInfo, externalRef string) (*payment.Order, error) {
	// Sandbox MP rejeita emails fora do domínio @testuser.com.
	if c.isSandbox {
		payer.Email = "test.buyer@testuser.com"
	}

	prefItems := make([]sdkpreference.ItemRequest, 0, len(items))
	for _, item := range items {
		prefItems = append(prefItems, sdkpreference.ItemRequest{
			Title:       item.Title,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			CurrencyID:  "BRL",
		})
	}

	payerReq := &sdkpreference.PayerRequest{
		Email:   payer.Email,
		Name:    firstWord(payer.Name),
		Surname: restWords(payer.Name),
	}
	if cpf := digitsOnly(payer.CPF); cpf != "" {
		payerReq.Identification = &sdkpreference.IdentificationRequest{
			Type:   taxIDType(payer.CPF),
			Number: cpf,
		}
	}

	req := sdkpreference.Request{
		ExternalReference: externalRef,
		Items:             prefItems,
		Payer:             payerReq,
		BackURLs: &sdkpreference.BackURLsRequest{
			Success: c.callbackBaseURL + "/v1/payments/result?status=success",
			Pending: c.callbackBaseURL + "/v1/payments/result?status=pending",
			Failure: c.callbackBaseURL + "/v1/payments/result?status=failure",
		},
		NotificationURL: c.notificationURL,
	}

	resp, err := c.preferenceClient.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderCreationFailed, err)
	}

	redirectURL := resp.InitPoint
	if c.isSandbox {
		redirectURL = resp.SandboxInitPoint
	}

	return &payment.Order{
		ID:                resp.ID,
		Status:            "created",
		RedirectURL:       redirectURL,
		ExternalReference: resp.ExternalReference,
	}, nil
}

// GetOrder não é usado na Preferences API — mantido por compatibilidade de interface.
func (c *SDKClient) GetOrder(_ context.Context, mpOrderID string) (*payment.Order, error) {
	return nil, payment.ErrOrderNotFound
}

// CancelOrder é no-op para Preferences: elas expiram automaticamente, sem cancelamento via API.
func (c *SDKClient) CancelOrder(_ context.Context, mpOrderID string) (*payment.Order, error) {
	return &payment.Order{ID: mpOrderID, Status: "cancelled"}, nil
}

// RefundOrder solicita estorno de um pagamento aprovado via SDK oficial do MP.
// O parâmetro deve ser o payment ID numérico (armazenado em MPPaymentID após webhook de confirmação).
func (c *SDKClient) RefundOrder(ctx context.Context, paymentID string, amount *string) (*payment.Order, error) {
	if paymentID == "" {
		return &payment.Order{}, nil
	}

	pid, err := strconv.Atoi(paymentID)
	if err != nil {
		return nil, fmt.Errorf("%w: payment ID inválido %q", payment.ErrOrderNotRefundable, paymentID)
	}

	var sdkErr error
	if amount != nil {
		amt, parseErr := strconv.ParseFloat(*amount, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: amount inválido %q", payment.ErrOrderNotRefundable, *amount)
		}
		_, sdkErr = c.refundClient.CreatePartialRefund(ctx, pid, amt)
	} else {
		_, sdkErr = c.refundClient.Create(ctx, pid)
	}
	if sdkErr != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderNotRefundable, sdkErr)
	}

	return &payment.Order{ID: paymentID, Status: "refunded"}, nil
}

// GetPayment busca os dados de um pagamento via GET /v1/payments/{paymentID}.
func (c *SDKClient) GetPayment(ctx context.Context, paymentID string) (*payment.Payment, error) {
	url := c.apiBaseURL + "/v1/payments/" + paymentID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, payment.ErrOrderNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get payment %s: HTTP %d", paymentID, resp.StatusCode)
	}

	var body struct {
		Status            string `json:"status"`
		StatusDetail      string `json:"status_detail"`
		ExternalReference string `json:"external_reference"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	return &payment.Payment{
		ID:                paymentID,
		Status:            body.Status,
		StatusDetail:      body.StatusDetail,
		ExternalReference: body.ExternalReference,
	}, nil
}

// --- helpers ---

// taxIDType retorna "CNPJ" se o tax ID tiver 14 dígitos, "CPF" caso contrário.
func taxIDType(taxID string) string {
	if len(digitsOnly(taxID)) == 14 {
		return "CNPJ"
	}
	return "CPF"
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' {
			return s[:i]
		}
	}
	return s
}

func restWords(s string) string {
	for i, r := range s {
		if r == ' ' {
			return strings.TrimSpace(s[i:])
		}
	}
	return ""
}
