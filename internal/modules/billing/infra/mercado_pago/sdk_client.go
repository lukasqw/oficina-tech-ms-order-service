package mercado_pago

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/mercadopago/sdk-go/pkg/config"
	sdkorder "github.com/mercadopago/sdk-go/pkg/order"
	sdkpayment "github.com/mercadopago/sdk-go/pkg/payment"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// SDKClient implementa payment.MercadoPagoClient usando o SDK oficial
// github.com/mercadopago/sdk-go (Orders API v1).
type SDKClient struct {
	orderClient     sdkorder.Client
	paymentClient   sdkpayment.Client
	notificationURL string
	callbackBaseURL string
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
		orderClient:     sdkorder.NewClient(cfg),
		paymentClient:  sdkpayment.NewClient(cfg),
		notificationURL: notificationURL,
		callbackBaseURL: strings.TrimRight(callbackBaseURL, "/"),
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
	if baseURL := strings.TrimSpace(os.Getenv("MP_BASE_URL")); baseURL != "" {
		opts = append(opts, config.WithHTTPClient(NewRewritingRequester(baseURL)))
	}

	return NewSDKClient(accessToken, notificationURL, callbackBaseURL, opts...)
}

// CreateOrder cria um Order "online" (Checkout Pro redirect) na Orders API do MP.
func (c *SDKClient) CreateOrder(ctx context.Context, items []payment.OrderItem, payer payment.PayerInfo, externalRef string) (*payment.Order, error) {
	sdkItems := make([]sdkorder.ItemsRequest, 0, len(items))
	for _, item := range items {
		sdkItems = append(sdkItems, sdkorder.ItemsRequest{
			Title:       item.Title,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   fmt.Sprintf("%.2f", item.UnitPrice),
		})
	}

	req := sdkorder.Request{
		Type:              "online",
		TotalAmount:       totalAmount(items),
		ExternalReference: externalRef,
		Currency:          "BRL",
		Items:             sdkItems,
		Payer: &sdkorder.PayerRequest{
			Email: payer.Email,
			Identification: &sdkorder.IdentificationRequest{
				Type:   taxIDType(payer.CPF),
				Number: digitsOnly(payer.CPF),
			},
		},
		Config: &sdkorder.ConfigRequest{
			Online: &sdkorder.OnlineConfigRequest{
				CallbackURL: c.notificationURL,
				SuccessURL:  c.callbackBaseURL + "/v1/payments/result?status=success",
				PendingURL:  c.callbackBaseURL + "/v1/payments/result?status=pending",
				FailureURL:  c.callbackBaseURL + "/v1/payments/result?status=failure",
			},
		},
	}

	resp, err := c.orderClient.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderCreationFailed, err)
	}
	return mapOrderResponse(resp), nil
}

// GetOrder busca o estado atual de um Order pelo ID do MP.
func (c *SDKClient) GetOrder(ctx context.Context, mpOrderID string) (*payment.Order, error) {
	resp, err := c.orderClient.Get(ctx, mpOrderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderNotFound, err)
	}
	return mapOrderResponse(resp), nil
}

// CancelOrder cancela um Order que ainda não foi pago.
func (c *SDKClient) CancelOrder(ctx context.Context, mpOrderID string) (*payment.Order, error) {
	resp, err := c.orderClient.Cancel(ctx, mpOrderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderNotCancellable, err)
	}
	return mapOrderResponse(resp), nil
}

// RefundOrder solicita estorno total (amount == nil) ou parcial de um Order já pago.
func (c *SDKClient) RefundOrder(ctx context.Context, mpOrderID string, amount *string) (*payment.Order, error) {
	var refundReq *sdkorder.RefundRequest
	if amount != nil {
		refundReq = &sdkorder.RefundRequest{}
	}
	resp, err := c.orderClient.Refund(ctx, mpOrderID, refundReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", payment.ErrOrderNotRefundable, err)
	}
	return mapOrderResponse(resp), nil
}

// GetPayment busca os dados de um pagamento interno pelo ID numérico do payment.
func (c *SDKClient) GetPayment(ctx context.Context, paymentID string) (*payment.Payment, error) {
	id, err := strconv.Atoi(paymentID)
	if err != nil {
		return nil, fmt.Errorf("payment id inválido %q: %w", paymentID, err)
	}
	resp, err := c.paymentClient.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &payment.Payment{
		ID:                paymentID,
		Status:            resp.Status,
		StatusDetail:      resp.StatusDetail,
		ExternalReference: resp.ExternalReference,
	}, nil
}

// --- helpers ---

func mapOrderResponse(resp *sdkorder.Response) *payment.Order {
	o := &payment.Order{
		ID:     resp.ID,
		Status: resp.Status,
	}
	if len(resp.Transactions.Payments) > 0 {
		pay := resp.Transactions.Payments[0]
		o.RedirectURL = pay.PaymentMethod.RedirectURL
		o.PaymentID = pay.ID
	}
	return o
}

func totalAmount(items []payment.OrderItem) string {
	var total float64
	for _, item := range items {
		total += item.UnitPrice * float64(item.Quantity)
	}
	return fmt.Sprintf("%.2f", total)
}

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
