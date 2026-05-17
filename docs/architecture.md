# Arquitetura Interna — ms-order-service

> Regras de negócio globais e ciclo de vida completo da OS: [docs/business-rules.md](./business-rules.md)
> Arquitetura macro da plataforma: [docs/architecture.md](../../docs/architecture.md)

---

## Estrutura de Pastas

```
internal/modules/
  billing/
    domain/payment/
      payment.go               ← Order, OrderItem, PayerInfo, Payment structs
                                  MercadoPagoClient interface (5 métodos)
                                  erros de domínio: ErrOrderNotFound, ErrOrderCreationFailed…
    application/usecases/
      create_payment_order.go  ← cria Order MP; usa snapshot de customer da OS
      handle_payment_webhook.go← GetOrder, approved→PAID, rejected→PAYMENT_REJECTED
      retry_payment.go         ← PAYMENT_REJECTED → novo mp_order_id → AWAITING_PAYMENT
      cancel_payment_order.go  ← CancelOrder no MP antes de CANCELED
      refund_payment_order.go  ← RefundOrder no MP antes de CANCELED (pós PAID)
      get_payment_status.go    ← retorna mp_order_id, payment_url, status
    infra/mercado_pago/
      sdk_client.go            ← SDKClient implementando MercadoPagoClient via SDK Go
      requester.go             ← RewritingRequester (override base URL para BDD via MP_BASE_URL)
      noop_client.go           ← 5 métodos noop para dev sem MP_ACCESS_TOKEN
      signature_validator.go   ← HMAC-SHA256 do webhook (manifest usa order_id)
    infra/http/handlers/
      payment_handler.go       ← GET /service-orders/{id}/payment, POST /retry-payment
      webhook_handler.go       ← POST /payments/mp-webhook (Orders API payload)
      result_handler.go        ← GET /payments/result (HTML pós-pagamento)

  service_order/
    domain/service_order/
      order_status.go          ← 12 estados + CanTransitionTo (inclui PAYMENT_REJECTED)
      service_order.go         ← SetCustomerSnapshot, CustomerEmail, CustomerName
    application/usecases/
      delete_service_order.go  ← injeta CancelPaymentOrder / RefundPaymentOrder
    application/saga/
      saga_orchestrator.go     ← CancelOrder inclui PAYMENT_REJECTED
    infra/persistence/
      service_order_model.go   ← MPOrderID (era MPPreferenceID), novas colunas
```

---

## Integração Mercado Pago — Orders API

### Restrição crítica do SDK

O SDK Go tem `urlBase = "https://api.mercadopago.com/v1/orders"` **hardcoded** em `pkg/order/client.go`. Não existe opção `WithBaseURL`. Para apontar para o mock BDD, usa-se a opção `config.WithHTTPClient(RewritingRequester)`:

```go
// Quando MP_BASE_URL está definido (ex: http://localhost:9999):
opts = append(opts, config.WithHTTPClient(mercado_pago.NewRewritingRequester(baseURL)))
mpClient, _ = mercado_pago.NewSDKClientFromEnv(opts...)
```

O `RewritingRequester` reescreve `r.URL.Host` para o host do mock antes de cada chamada HTTP.

### Interface MercadoPagoClient

```go
type MercadoPagoClient interface {
    CreateOrder(ctx, serviceOrderID string, items []OrderItem, payer PayerInfo, externalRef string) (*Order, error)
    GetOrder(ctx, mpOrderID string) (*Order, error)
    CancelOrder(ctx, mpOrderID string) (*Order, error)
    RefundOrder(ctx, mpOrderID string, amount *string) (*Order, error)
    GetPayment(ctx, paymentID string) (*Payment, error)
}
```

- `amount = nil` em `RefundOrder` → estorno total
- `CancelOrder` trata 404/410 como idempotente (order já cancelado)

### Wiring em `cmd/api/main.go`

```go
var mpClient payment.MercadoPagoClient
if os.Getenv("MP_ACCESS_TOKEN") == "" {
    mpClient = mercado_pago.NewNoopClient()
} else {
    opts := []config.Option{}
    if baseURL := os.Getenv("MP_BASE_URL"); baseURL != "" {
        opts = append(opts, config.WithHTTPClient(mercado_pago.NewRewritingRequester(baseURL)))
    }
    mpClient, err = mercado_pago.NewSDKClientFromEnv(opts...)
}
```

---

## Fluxo de Pagamento

```
advance (COMPLETED)
    └─ CreatePaymentOrder
           ├─ le snapshot customer_email, customer_name da OS (sem chamar MS1)
           ├─ SDK.Create(order.Request{type:"online", items, payer, config.Online.*})
           │       config.Online.CallbackURL = MP_NOTIFICATION_URL
           │       config.Online.SuccessURL  = MP_CALLBACK_BASE_URL + /payments/result?status=success
           │       config.Online.PendingURL  = MP_CALLBACK_BASE_URL + /payments/result?status=pending
           │       config.Online.FailureURL  = MP_CALLBACK_BASE_URL + /payments/result?status=failure
           ├─ persiste mp_order_id, payment_url
           └─ OS → AWAITING_PAYMENT

POST /payments/mp-webhook {type:"order", data:{id: order_id}}
    └─ WebhookHandler
           ├─ valida HMAC-SHA256 (manifest: id:{order_id};request-id:{req-id};ts:{ts};)
           └─ HandlePaymentWebhook
                  ├─ SDK.GetOrder(order_id)
                  ├─ payment.status == "approved" → OS → PAID, persiste mp_payment_id
                  ├─ payment.status == "rejected" → OS → PAYMENT_REJECTED, persiste payment_rejection_reason
                  └─ pending/in_process → sem mudança de status
```

---

## Endpoints HTTP

Porta: `8082`

| Método | Path | Auth | Descrição |
|--------|------|------|-----------|
| `POST` | `/service-orders` | USER+ | Criar OS; snapshot customer via REST ao MS1 |
| `GET` | `/service-orders` | CUSTOMER+ | Listar OS |
| `GET` | `/service-orders/{id}` | USER+ | Buscar OS por ID |
| `GET` | `/service-orders/{id}/history` | USER+ | Histórico DynamoDB |
| `PUT` | `/service-orders/{id}` | USER+ | Atualizar OS |
| `POST` | `/service-orders/{id}/advance` | USER+ | Avançar status |
| `POST` | `/service-orders/{id}/authorize` | CUSTOMER+ | Aprovar/rejeitar autorização |
| `DELETE` | `/service-orders/{id}` | MANAGER+ | Cancelar OS (MP Cancel/Refund + saga) |
| `GET` | `/service-orders/{id}/payment` | USER+ | Retorna `mp_order_id`, `payment_url`, `status` |
| `POST` | `/service-orders/{id}/retry-payment` | USER+ | Retry para OS em `PAYMENT_REJECTED` |
| `POST` | `/payments/mp-webhook` | Público (HMAC) | Webhook Orders API do Mercado Pago |
| `GET` | `/payments/result` | Público | HTML pós-pagamento (`?status=success\|pending\|failure`) |
| `GET` | `/health` | Público | Health check |

---

## Schema PostgreSQL — `service_orders` (campos MP)

Após migration `003_mp_orders_api_migration.sql`:

| Coluna | Tipo | Origem |
|--------|------|--------|
| `mp_order_id` | `VARCHAR(255)` | `order.Response.ID` (renomeado de `mp_preference_id`) |
| `mp_payment_id` | `VARCHAR(255)` | `Transactions.Payments[0].ID` (quando aprovado) |
| `payment_url` | `TEXT` | `Transactions.Payments[0].PaymentMethod.RedirectURL` |
| `mp_order_status` | `VARCHAR(50)` | Cache do `order.Response.Status` |
| `payment_rejection_reason` | `VARCHAR(255)` | `Transactions.Payments[0].StatusDetail` (quando rejected) |
| `customer_email` | `VARCHAR(255)` | Snapshot do MS1 na criação da OS |
| `customer_name` | `VARCHAR(255)` | Snapshot do MS1 na criação da OS |

---

## Mock BDD (`bdd/mockmp/`)

O mock (`localhost:9999`) implementa a Orders API em memória:

| Endpoint | Propósito |
|----------|-----------|
| `POST /v1/orders` | Cria order; retorna `id = "order-{external_reference}"` |
| `GET /v1/orders/{id}` | Retorna estado atual |
| `POST /v1/orders/{id}/cancel` | Define `payment_status = "cancelled"` |
| `POST /v1/orders/{id}/refund` | Define `payment_status = "refunded"` |
| `POST /__mock/orders/{id}` | Admin: configura `payment_status` e `status_detail` |
| `POST /__mock/orders/{id}/trigger` | Admin: dispara webhook ao MS2 com HMAC válido |
| `GET /__mock/orders` | Admin: lista todos os orders (para assertions BDD) |
| `POST /__mock/reset` | Admin: limpa estado |

O webhook disparado pelo mock usa o mesmo formato da Orders API:
```json
{"type": "order", "action": "order.updated", "data": {"id": "<order_id>"}}
```
com headers `x-signature: ts={ts},v1={hmac}` e `x-request-id`.
