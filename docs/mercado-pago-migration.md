# Plano de Migração — Mercado Pago SDK Go (Orders API)

> **Status**: Proposta de planejamento — pendente aprovação para execução
> **SDK alvo**: `github.com/mercadopago/sdk-go@v1.8.1`
> **API alvo**: Orders API (`pkg/order`) — endpoint `https://api.mercadopago.com/v1/orders`
> **API atual**: Preferences (Checkout Pro) via HTTP nativo
> **Data**: 2026-05-17

---

## 1. Motivação

A integração atual com Mercado Pago é feita via `net/http` puro contra a API de **Preferences** (Checkout Pro). A meta é migrar para o **SDK oficial Go** usando a **Orders API** (nova, lançada em 2024) que combina preferência e pagamento em um único recurso. Benefícios:

- Tipos fortes e autocomplete (Request/Response struct do SDK)
- Manutenção pelo time oficial MP (atualizações de API, novos campos)
- Retry, timeout e headers gerenciados pelo SDK
- Suporte a Industry Fields, 3DS, Capture, Refund nativos
- Alinhamento ao roadmap MP (Preferences ainda funciona, mas Orders é a recomendação atual)

---

## 1.1 Decisões Tomadas

| # | Decisão | Detalhe |
|---|---------|---------|
| 1 | **Migration em 1 step** | `RENAME COLUMN` direto. Coordenar janela de deploy curta (downtime ≤ 1 min). Deploy backend ANTES da migration falha — usar feature flag `MP_USE_ORDERS_API` ou deploy atômico via Helm |
| 2 | **Novo status `PAYMENT_REJECTED`** | Não é terminal: permite retentativa do pagamento ou cancelamento explícito da OS. Adicionado à máquina de estados |
| 3 | **URLs de callback/success/failure são NOSSAS** | MS2 serve 3 endpoints HTML simples para o redirect pós-pagamento; `callback_url` aponta para o webhook MP (`POST /payments/mp-webhook`) |
| 4 | **Snapshot do customer na OS** | Caminho mais performático: adicionar colunas `customer_email`, `customer_cpf`, `customer_name` em `service_orders` (preenchidas via REST sync com MS1 **uma vez** na criação da OS). Zero chamadas extras no momento do pagamento |
| 5 | **Implementar Cancel/Refund agora** | Liga ao saga de compensação: `Cancel` antes do pagamento, `Refund` após `PAID`. Substitui/complementa o `CANCEL_CONFIRMED` do saga de estoque |

---

## 1.2 Novo Estado `PAYMENT_REJECTED` na Máquina de Estados

```
... COMPLETED → AWAITING_PAYMENT ─┬─→ PAID → DELIVERED
                                  │
                                  └─→ PAYMENT_REJECTED ─┬─→ AWAITING_PAYMENT (retry)
                                                        └─→ CANCELED (com refund/release de estoque)
```

- `PAYMENT_REJECTED`: **não é terminal**
- Transições permitidas a partir de `PAYMENT_REJECTED`:
  - `AWAITING_PAYMENT`: nova tentativa de pagamento via `POST /service-orders/{id}/retry-payment` (cria novo `mp_order_id`)
  - `CANCELED`: cancelamento explícito; dispara `CANCEL_CONFIRMED` no saga de estoque
- E-mail notificando rejeição é enviado ao cliente
- Frontend/cliente recebe URL de retry no payload do webhook handler

---

## 1.3 URLs de Redirect/Callback (Detalhamento)

| URL no SDK | Aponta para | Quem implementa | Propósito |
|-----------|-------------|-----------------|-----------|
| `Config.Online.CallbackURL` | `https://api.<dominio>/v1/payments/mp-webhook` | MS2 (já existe) | Webhook server-to-server do MP |
| `Config.Online.SuccessURL` | `https://api.<dominio>/v1/payments/result?status=success&order={id}` | MS2 (novo endpoint) | Página HTML simples confirmando recebimento |
| `Config.Online.PendingURL` | `https://api.<dominio>/v1/payments/result?status=pending&order={id}` | MS2 (novo endpoint) | Página HTML simples informando pagamento pendente |
| `Config.Online.FailureURL` | `https://api.<dominio>/v1/payments/result?status=failure&order={id}` | MS2 (novo endpoint) | Página HTML simples informando falha e oferecendo retry |

> **Decisão de implementação**: criar 1 handler único `GET /payments/result` que aceita query string e renderiza HTML estático (template Go embedado). Isolado, sem dependência de frontend, fácil de evoluir.

Variáveis novas no `.env.example`:

```bash
MP_CALLBACK_BASE_URL=https://api.oficinatech.com  # Base pública para callbacks MP
```

---

## 2. Estado Atual

### 2.1 Componentes a substituir

| Arquivo | Função | Ação |
|---------|--------|------|
| `internal/modules/billing/infra/mercado_pago/client.go` | HTTP client custom + retry 3x | **Substituir** por wrapper do SDK |
| `internal/modules/billing/infra/mercado_pago/dtos.go` | DTOs JSON manuais para `/checkout/preferences` | **Remover** (SDK já tem `order.Request/Response`) |
| `internal/modules/billing/infra/mercado_pago/noop_client.go` | Fallback quando `MP_ACCESS_TOKEN` vazio | **Manter** (não depende do transporte) |
| `internal/modules/billing/infra/mercado_pago/signature_validator.go` | HMAC-SHA256 do webhook | **Manter** (SDK não fornece helper) |
| `internal/modules/billing/domain/payment/repository.go` (interface `MercadoPagoClient`) | Port para o adapter | **Renomear** método `CreatePreference` → `CreateOrder`; ajustar entidade `Preference` → `Order` |
| `internal/modules/billing/application/usecases/create_payment_preference.go` | Cria preferência na transição COMPLETED | **Renomear** para `create_payment_order.go`; ajustar para Orders API |
| `internal/modules/billing/application/usecases/handle_payment_webhook.go` | Processa webhook MP | **Atualizar** lógica: payload da Orders API usa `data.id = order_id` (não payment_id) |
| `migrations/002_payment_fields.sql` | Adiciona `mp_preference_id`, `mp_payment_id`, `payment_url` | **Nova migration 003**: renomear `mp_preference_id` → `mp_order_id` |
| `bdd/mockmp/main.go` | Mock HTTP MP (porta 9999) | **Adaptar** endpoints de `/checkout/preferences` para `/v1/orders` |

### 2.2 Tabela `service_orders` — campos MP + snapshot do customer

| Antes | Depois | Origem |
|-------|--------|--------|
| `mp_preference_id VARCHAR(255)` | `mp_order_id VARCHAR(255)` | SDK `order.Response.ID` |
| `mp_payment_id VARCHAR(255)` | `mp_payment_id VARCHAR(255)` (mantém) | `Transactions.Payments[0].ID` |
| `payment_url TEXT` | `payment_url TEXT` (mantém) | `Transactions.Payments[0].PaymentMethod.RedirectURL` |
| — | `mp_order_status VARCHAR(50)` (**novo**) | `order.Response.Status` (cache do último status conhecido) |
| — | `payment_rejection_reason VARCHAR(255)` (**novo**) | `Transactions.Payments[0].StatusDetail` (se rejeitado) |
| — | `customer_email VARCHAR(255)` (**novo, snapshot**) | REST sync com MS1 na criação da OS |
| — | `customer_cpf VARCHAR(20)` (**novo, snapshot**) | REST sync com MS1 na criação da OS |
| — | `customer_name VARCHAR(255)` (**novo, snapshot**) | REST sync com MS1 na criação da OS |

**Por que snapshot?** Performance — o `create order` no MP precisa do email + CPF do `Payer`. Sem snapshot, cada criação de pagamento gera uma chamada REST adicional para o MS1. Como esses dados raramente mudam após a criação da OS, o snapshot é a abordagem mais performática e desacopla o MS2 de chamadas síncronas extras ao MS1 no momento crítico do pagamento.

---

## 3. Análise Técnica do SDK

### 3.1 Inicialização

```go
import (
    "github.com/mercadopago/sdk-go/pkg/config"
    "github.com/mercadopago/sdk-go/pkg/order"
)

cfg, err := config.New(accessToken)
client := order.NewClient(cfg)
```

### 3.2 `order.Request` — campos relevantes para Checkout Pro online

| Campo | Tipo | Mapeamento com OS |
|-------|------|-------------------|
| `Type` | `string` | `"online"` |
| `TotalAmount` | `string` | Soma dos `service_order_items.unit_price * quantity` formatada (`"1234.56"`) |
| `ExternalReference` | `string` | `service_order.id` (UUID) |
| `Currency` | `string` | `"BRL"` |
| `Description` | `string` | `"Ordem de serviço #<id curto>"` |
| `Items[]` | `[]ItemsRequest` | Cada `service_order_item` → `{Title, UnitPrice, Quantity, Description}` |
| `Payer.Email` | `string` | Email do `customer` (snapshot armazenado na OS ou via REST sync com MS1) |
| `Payer.Identification.{Type,Number}` | `string` | CPF do customer |
| `Config.Online.CallbackURL` | `string` | URL pública do MS2 (notificações + redirect base) |
| `Config.Online.SuccessURL` | `string` | URL de retorno após pagamento aprovado |
| `Config.Online.PendingURL` | `string` | URL de retorno para pagamento pendente |
| `Config.Online.FailureURL` | `string` | URL de retorno para pagamento rejeitado |

### 3.3 `order.Response` — campos a persistir

| Campo SDK | Coluna DB |
|-----------|-----------|
| `ID` | `service_orders.mp_order_id` |
| `Status` | (em memória — controla transição de OS) |
| `Transactions.Payments[0].ID` | `service_orders.mp_payment_id` (quando disponível) |
| `Transactions.Payments[0].PaymentMethod.RedirectURL` | `service_orders.payment_url` |

### 3.4 Restrição crítica — Base URL hardcoded

O SDK tem `urlBase = "https://api.mercadopago.com/v1/orders"` **hardcoded em `pkg/order/client.go`**. Não existe `WithBaseURL` em `config_options.go`. As únicas opções de configuração são:

- `WithHTTPClient(requester.Requester)` — injeta um Requester custom
- `WithCorporationID`, `WithIntegratorID`, `WithPlatformID`, `WithExpandNodes`

**Implicação para BDD**: o mock atual (porta 9999) **não pode** ser acionado via `MP_BASE_URL`. Soluções (em ordem de preferência):

1. **(Recomendado)** Implementar `requester.Requester` custom que reescreve `r.URL.Host` para `localhost:9999` quando `MP_BASE_URL` está setado. Injeta via `config.New(token, config.WithHTTPClient(customRequester))`.
2. Manter mock atual mas atualizar paths para `/v1/orders/*`.
3. Substituir mock HTTP por mock Go da interface `MercadoPagoClient` em testes (não cobre fluxo HTTP real do SDK).

---

## 4. Mudanças por Camada

### 4.1 `domain/payment`

```go
// ANTES
type Preference struct {
    ID      string
    InitURL string
}

// DEPOIS
type Order struct {
    ID            string  // mp_order_id
    Status        string  // created, processed, action_required, completed...
    PaymentID     string  // primeiro payment do order, se houver
    RedirectURL   string  // URL de checkout (init_point equivalente)
}
```

Interface `MercadoPagoClient`:

```go
type MercadoPagoClient interface {
    CreateOrder(ctx context.Context, orderID string, items []OrderItem, payer PayerInfo, externalRef string) (*Order, error)
    GetOrder(ctx context.Context, orderID string) (*Order, error)
    CancelOrder(ctx context.Context, mpOrderID string) (*Order, error)              // NOVO
    RefundOrder(ctx context.Context, mpOrderID string, amount *string) (*Order, error) // NOVO (amount=nil = total)
    GetPayment(ctx context.Context, paymentID string) (*Payment, error)             // mantém para webhook
}
```

Erros de domínio: manter `ErrMissingAccessToken`, `ErrInvalidWebhookSignature`; adicionar:
- `ErrOrderNotFound`
- `ErrOrderCreationFailed`
- `ErrOrderNotCancellable` (tentativa de cancelar pedido já pago)
- `ErrOrderNotRefundable` (tentativa de estornar pedido não pago)

### 4.2 `application/usecases`

#### Usecases existentes a atualizar

- `create_payment_preference.go` → `create_payment_order.go`
  - Recebe `service_order_id`, lê items E customer snapshot (já persistido na OS), monta `OrderItem[]` e `PayerInfo` **sem chamar MS1**
  - Chama `client.CreateOrder()` → salva `mp_order_id`, `payment_url`, `mp_order_status`, avança OS para `AWAITING_PAYMENT`

- `handle_payment_webhook.go` — payload da Orders API: `{type, action, data: {id}}` onde `data.id = order_id`. Fluxo:
  1. Validar HMAC do webhook
  2. Chamar `client.GetOrder(orderID)`
  3. Atualizar `mp_order_status` no DB
  4. Inspecionar `order.Status` e `order.Transactions.Payments[0].Status`:
     - **`approved`** → OS → `PAID`, persiste `mp_payment_id`, envia email confirmação
     - **`rejected`** → OS → `PAYMENT_REJECTED`, persiste `payment_rejection_reason`, envia email com URL de retry
     - **`pending` / `in_process`** → mantém OS em `AWAITING_PAYMENT`, log apenas
     - **`cancelled`** (pelo MP) → OS → `CANCELED`, dispara `CANCEL_CONFIRMED` no saga de estoque

#### Usecases novos

- `retry_payment.go` — `POST /service-orders/{id}/retry-payment`
  - Aplicável quando OS está em `PAYMENT_REJECTED`
  - Cria novo `mp_order_id` (sobrescreve antigo após preservar histórico no DynamoDB `order_history`)
  - Volta OS para `AWAITING_PAYMENT`

- `cancel_payment_order.go` — usecase interno (não exposto via HTTP)
  - Chamado automaticamente quando OS é cancelada em `AWAITING_PAYMENT` ou `PAYMENT_REJECTED`
  - Chama `client.CancelOrder(mp_order_id)` para evitar cobrança futura
  - Não falha o cancelamento se MP retornar 404/410 (idempotência)

- `refund_payment_order.go` — usecase interno
  - Chamado quando OS é cancelada após `PAID`
  - Chama `client.RefundOrder(mp_order_id, nil)` (estorno total)
  - Persiste resultado em `service_order_histories`
  - Compensa também o estoque via `CANCEL_CONFIRMED` no saga

#### Integração com o Saga de Cancelamento

O fluxo atual de cancelamento de OS dispara `CANCEL_RESERVED` ou `CANCEL_CONFIRMED` no MS3. Com Cancel/Refund do MP, o orquestrador passa a ter mais um step:

```
[Cancelamento de OS em AWAITING_PAYMENT]
1. cancel_payment_order  → MP CancelOrder
2. CANCEL_RESERVED ou CANCEL_CONFIRMED → MS3 libera estoque
3. OS → CANCELED

[Cancelamento de OS em PAID]
1. refund_payment_order  → MP RefundOrder
2. CANCEL_CONFIRMED → MS3 libera estoque
3. OS → CANCELED (com nota de refund processado)
```

Falha no MP Cancel/Refund **bloqueia** a transição para `CANCELED` (não queremos cancelar a OS sem desfazer o pagamento). Saga retenta via SQS DLQ ou marcação manual.

### 4.3 `infra/mercado_pago`

**Novo arquivo**: `sdk_client.go`

```go
type sdkClient struct {
    client order.Client
    cfg    *config.Config
    notificationURL string
}

func NewSDKClient(accessToken string, notificationURL string, opts ...config.Option) (*sdkClient, error) {
    cfg, err := config.New(accessToken, opts...)
    if err != nil { return nil, err }
    return &sdkClient{
        client: order.NewClient(cfg),
        cfg:    cfg,
        notificationURL: notificationURL,
    }, nil
}

func (c *sdkClient) CreateOrder(ctx context.Context, orderID string, items []OrderItem, payer PayerInfo, externalRef string) (*Order, error) {
    req := order.Request{
        Type: "online",
        TotalAmount: sumItems(items),
        ExternalReference: externalRef,
        Currency: "BRL",
        Items: toSDKItems(items),
        Payer: &order.PayerRequest{
            Email: payer.Email,
            Identification: &order.IdentificationRequest{
                Type: "CPF", Number: payer.CPF,
            },
        },
        Config: &order.ConfigRequest{
            Online: &order.OnlineConfigRequest{
                CallbackURL: c.notificationURL,
                SuccessURL:  c.notificationURL + "/success",
                PendingURL:  c.notificationURL + "/pending",
                FailureURL:  c.notificationURL + "/failure",
            },
        },
    }
    resp, err := c.client.Create(ctx, req)
    if err != nil { return nil, fmt.Errorf("mp create order: %w", err) }
    return mapOrderResponse(resp), nil
}
```

**Novo arquivo**: `requester.go` (Requester custom para BDD)

```go
type rewritingRequester struct {
    inner requester.Requester
    baseHost string  // ex: localhost:9999
}

func (r *rewritingRequester) Do(req *http.Request) (*http.Response, error) {
    if r.baseHost != "" {
        req.URL.Host = r.baseHost
        req.URL.Scheme = "http"
    }
    return r.inner.Do(req)
}
```

Ativa via `MP_BASE_URL=http://localhost:9999` no docker-compose.e2e.yml.

### 4.4 `infra/http/handlers`

- `webhook_handler.go`: payload da Orders API tem schema diferente:
  ```json
  {"type":"order","action":"order.updated","data":{"id":"<order_id>"}}
  ```
- Adaptar parse e validação (assinatura HMAC permanece igual; manifest usa `data.id` que agora é order_id, não payment_id).

### 4.5 Wiring em `cmd/api/main.go`

```go
var mpClient payment.MercadoPagoClient
if os.Getenv("MP_ACCESS_TOKEN") == "" {
    mpClient = mercado_pago.NewNoopClient()
} else {
    opts := []config.Option{}
    if baseURL := os.Getenv("MP_BASE_URL"); baseURL != "" {
        opts = append(opts, config.WithHTTPClient(mercado_pago.NewRewritingRequester(baseURL)))
    }
    mpClient, err = mercado_pago.NewSDKClient(
        os.Getenv("MP_ACCESS_TOKEN"),
        os.Getenv("MP_NOTIFICATION_URL"),
        opts...,
    )
    if err != nil { log.Fatalf("init mp client: %v", err) }
}
```

---

## 5. Schema — Migration 003

`migrations/003_mp_orders_api_migration.sql` (1 step, executada com janela curta de downtime):

```sql
BEGIN;

-- Rename do campo principal
ALTER TABLE service_orders RENAME COLUMN mp_preference_id TO mp_order_id;

-- Cache do status do order MP (evita GetOrder repetido)
ALTER TABLE service_orders ADD COLUMN mp_order_status VARCHAR(50);

-- Motivo de rejeição (status_detail do MP)
ALTER TABLE service_orders ADD COLUMN payment_rejection_reason VARCHAR(255);

-- Snapshot do customer para o Payer do MP (evita REST sync com MS1 a cada pagamento)
ALTER TABLE service_orders ADD COLUMN customer_email VARCHAR(255);
ALTER TABLE service_orders ADD COLUMN customer_cpf VARCHAR(20);
ALTER TABLE service_orders ADD COLUMN customer_name VARCHAR(255);

-- Backfill: para OS existentes, popular snapshot via JOIN com cache local de customers (se houver)
-- ou deixar NULL — OSes antigas em estado não pagável não precisam dos campos
-- (a coluna NULL não bloqueia leituras)

COMMIT;
```

Atualizar `service_order_model.go` (GORM):

```go
MPOrderID              *string `gorm:"column:mp_order_id"`           // antes: MPPreferenceID
MPPaymentID            *string `gorm:"column:mp_payment_id"`         // mantém
MPOrderStatus          *string `gorm:"column:mp_order_status"`       // NOVO
PaymentURL             *string `gorm:"column:payment_url"`           // mantém
PaymentRejectionReason *string `gorm:"column:payment_rejection_reason"` // NOVO
CustomerEmail          *string `gorm:"column:customer_email"`        // NOVO
CustomerCPF            *string `gorm:"column:customer_cpf"`          // NOVO
CustomerName           *string `gorm:"column:customer_name"`         // NOVO
```

### 5.1 Adicionar status `PAYMENT_REJECTED` à máquina de estados

Atualizar `internal/modules/service_order/domain/service_order/order_status.go`:

```go
const (
    StatusReceived             OrderStatus = "RECEIVED"
    StatusDiagnosing           OrderStatus = "DIAGNOSING"
    StatusPendingAuthorization OrderStatus = "PENDING_AUTHORIZATION"
    StatusAuthorized           OrderStatus = "AUTHORIZED"
    StatusInProgress           OrderStatus = "IN_PROGRESS"
    StatusCompleted            OrderStatus = "COMPLETED"
    StatusAwaitingPayment      OrderStatus = "AWAITING_PAYMENT"
    StatusPaymentRejected      OrderStatus = "PAYMENT_REJECTED"  // NOVO
    StatusPaid                 OrderStatus = "PAID"
    StatusDelivered            OrderStatus = "DELIVERED"
    StatusCanceled             OrderStatus = "CANCELED"
    StatusAuthorizationDenied  OrderStatus = "AUTHORIZATION_DENIED"
)
```

Transições válidas a adicionar na função `CanTransitionTo`:
- `AWAITING_PAYMENT` → `PAYMENT_REJECTED`
- `PAYMENT_REJECTED` → `AWAITING_PAYMENT` (retry)
- `PAYMENT_REJECTED` → `CANCELED`

Atualizar `business-rules.md` (root) com o novo estado no stateDiagram.

---

## 6. Mock BDD (`bdd/mockmp/`)

### 6.1 Endpoints novos

| Antes | Depois | Propósito |
|-------|--------|-----------|
| `POST /checkout/preferences` | `POST /v1/orders` | Cria order |
| `GET /v1/payments/{id}` | `GET /v1/orders/{id}` | Busca order |
| — | `POST /v1/orders/{id}/cancel` | **Cancel** order (novo) |
| — | `POST /v1/orders/{id}/refund` | **Refund** order (novo) |
| `POST /__mock/payments/{id}` | `POST /__mock/orders/{id}` | Admin: define status do order e payment associado |
| — | `POST /__mock/orders/{id}/trigger-webhook` | Admin: dispara webhook manualmente (novo) |
| `POST /__mock/reset` | `POST /__mock/reset` | Limpa estado (mantém) |

### 6.2 Response format

```json
{
  "id": "order-<external_reference>",
  "status": "created",
  "external_reference": "<os_id>",
  "transactions": {
    "payments": [{
      "id": "pay-<order_id>",
      "status": "pending",
      "status_detail": null,
      "payment_method": {
        "redirect_url": "https://mock.mercadopago.local/checkout/order-<id>"
      }
    }]
  }
}
```

### 6.3 Status simuláveis pelo mock

O endpoint `POST /__mock/orders/{id}` deve aceitar:

```json
{
  "status": "processed",
  "payment_status": "approved",          // approved, rejected, pending, cancelled, refunded
  "status_detail": "cc_rejected_insufficient_amount"
}
```

### 6.4 Webhook do mock

Mock dispara `POST <MP_NOTIFICATION_URL>` com payload:

```json
{"type":"order","action":"order.updated","data":{"id":"<order_id>"}}
```

Assinatura HMAC válida usando `MP_WEBHOOK_SECRET`. Trigger:
- Automaticamente após `POST /__mock/orders/{id}` se `status_detail` mudou
- Manualmente via `POST /__mock/orders/{id}/trigger-webhook`

---

## 7. Variáveis de Ambiente

| Variável | Antes | Depois | Notas |
|----------|-------|--------|-------|
| `MP_ACCESS_TOKEN` | ✅ | ✅ | Sem mudança |
| `MP_WEBHOOK_SECRET` | ✅ | ✅ | Sem mudança |
| `MP_NOTIFICATION_URL` | ✅ | ✅ | Usada em `Config.Online.CallbackURL` (webhook do MS2) |
| `MP_CALLBACK_BASE_URL` | — | ✅ (**NOVA**) | Base pública do MS2 (ex: `https://api.oficinatech.com`). Usada para construir SuccessURL/PendingURL/FailureURL |
| `MP_BASE_URL` | ✅ (BDD) | ✅ (BDD) | Continua sendo o host de override para o mock; agora injetado via Requester custom |

---

## 8. Cenários BDD (`features/payment_flow.feature`)

Os 3 cenários existentes ganham ajustes + 4 novos cenários:

### 8.1 Cenários existentes (atualizados)

| Cenário | Mudança |
|---------|---------|
| Pagamento aprovado | Mock retorna order com `payments[0].status = "approved"`; webhook recebe `data.id = order_id`; OS → PAID |
| Pagamento rejeitado | Mock retorna order com `payments[0].status = "rejected"`; OS → **PAYMENT_REJECTED** (novo status); email com URL de retry enviado |
| Assinatura inválida | Webhook rejeitado com 401 (sem mudança) |

### 8.2 Cenários novos

| Cenário | Descrição |
|---------|-----------|
| **Retry após rejeição** | OS em `PAYMENT_REJECTED` → `POST /service-orders/{id}/retry-payment` → novo `mp_order_id` criado → OS volta a `AWAITING_PAYMENT` |
| **Cancelamento em AWAITING_PAYMENT** | Cancela OS aguardando pagamento → MS2 chama MP Cancel → MS3 libera estoque (`CANCEL_RESERVED`) → OS → `CANCELED` |
| **Cancelamento após PAID (Refund)** | Cancela OS já paga → MS2 chama MP Refund → MS3 libera estoque (`CANCEL_CONFIRMED`) → OS → `CANCELED` com nota de refund |
| **Falha no Refund bloqueia cancelamento** | MP retorna erro no Refund → transição para `CANCELED` é abortada → OS permanece em `PAID` com flag de erro |

---

## 9. Fases de Execução

### Fase 1 — Setup (≤ 2h)
- [x] Adicionar SDK ao `go.mod` (já feito: v1.8.1)
- [ ] Atualizar `.env.example` com `MP_CALLBACK_BASE_URL` e comentário sobre `MP_BASE_URL`
- [ ] Criar migration `003_mp_orders_api_migration.sql` (rename + 5 colunas novas)

### Fase 2 — Domain (≤ 3h)
- [ ] Renomear `Preference` → `Order` em `domain/payment/payment.go`
- [ ] Adicionar `OrderItem`, `PayerInfo`, `Payment` structs
- [ ] Atualizar interface `MercadoPagoClient` (5 métodos: Create, Get, Cancel, Refund, GetPayment)
- [ ] Adicionar erros de domínio: `ErrOrderNotFound`, `ErrOrderCreationFailed`, `ErrOrderNotCancellable`, `ErrOrderNotRefundable`
- [ ] Adicionar `StatusPaymentRejected` em `order_status.go` + atualizar `CanTransitionTo`

### Fase 3 — Adapter SDK (≤ 4h)
- [ ] Criar `sdk_client.go` implementando `MercadoPagoClient` (5 métodos)
- [ ] Criar `requester.go` (rewriting requester para BDD usando `config.WithHTTPClient`)
- [ ] Atualizar `noop_client.go` com os 5 métodos
- [ ] Remover `client.go` e `dtos.go` antigos
- [ ] Manter `signature_validator.go` (HMAC) com manifest adaptado para `data.id = order_id`
- [ ] Mapper helpers: `toSDKItems`, `toSDKPayer`, `mapOrderResponse`, `mapPaymentResponse`

### Fase 4 — Application (≤ 5h)
- [ ] Renomear `create_payment_preference.go` → `create_payment_order.go` (lê snapshot de customer; sem REST sync com MS1)
- [ ] Ajustar `handle_payment_webhook.go` para payload da Orders API com tratamento de `approved`, `rejected`, `pending`, `cancelled`
- [ ] Criar `retry_payment.go` (novo usecase)
- [ ] Criar `cancel_payment_order.go` (interno, chamado pelo cancelamento de OS)
- [ ] Criar `refund_payment_order.go` (interno, chamado pelo cancelamento pós-PAID)
- [ ] Atualizar `get_payment_status.go` para usar `GetOrder`

### Fase 5 — Customer Snapshot na OS (≤ 3h)
- [ ] Atualizar `create_service_order.go` usecase para fazer REST sync com MS1 e persistir `customer_email`, `customer_cpf`, `customer_name`
- [ ] Atualizar adapter MS1 com `GetCustomerFull(id)` retornando email/cpf/name
- [ ] Atualizar contrato GET `/customers/{id}` no MS1 se necessário (validar com agente do MS1)
- [ ] Testes unitários do create_service_order

### Fase 6 — Persistence & HTTP (≤ 3h)
- [ ] Atualizar `service_order_model.go` (5 colunas novas + rename)
- [ ] Atualizar handler do webhook para novo schema
- [ ] Criar handler `GET /payments/result?status=...&order=...` com template HTML embedado
- [ ] Criar handler `POST /service-orders/{id}/retry-payment`
- [ ] Atualizar wiring em `cmd/api/main.go` (SDK client + Requester custom condicional)

### Fase 7 — Saga & Cancelamento (≤ 4h)
- [ ] Atualizar usecase `cancel_service_order.go` para chamar `cancel_payment_order` ou `refund_payment_order` antes de disparar saga de estoque
- [ ] Garantir que falha no MP Cancel/Refund **bloqueia** transição para `CANCELED`
- [ ] Atualizar transições da máquina de estados para considerar resultado do MP
- [ ] Atualizar features BDD `service_order_lifecycle.feature` e `saga_compensation.feature` com novos steps

### Fase 8 — Mock BDD (≤ 3h)
- [ ] Atualizar `bdd/mockmp/main.go` com endpoints `/v1/orders/*` (Create, Get, Cancel, Refund)
- [ ] Atualizar admin endpoints `POST /__mock/orders/{id}` (status + payment_status + status_detail)
- [ ] Implementar trigger automático e manual de webhook
- [ ] Validar que `MP_BASE_URL` com Requester custom funciona em testes E2E

### Fase 9 — Testes BDD & Unitários (≤ 5h)
- [ ] Atualizar `features/payment_flow.feature` com cenários novos (retry, cancel, refund, falha refund)
- [ ] Testes unitários: novos usecases (retry, cancel, refund)
- [ ] Testes unitários: `sdk_client.go` (com mock do `order.Client`)
- [ ] Testes da máquina de estados: novas transições com `PAYMENT_REJECTED`
- [ ] Rodar BDD completo e ajustar até passar
- [ ] Manter cobertura ≥ 80%

### Fase 10 — Docs (≤ 3h)
- [ ] Atualizar `README.md` do MS2 (endpoints novos, status PAYMENT_REJECTED, fluxo Cancel/Refund)
- [ ] Atualizar `docs/architecture.md` do MS2 (fluxo MP atualizado)
- [ ] Atualizar `docs/business-rules.md` do MS2 (regras de PAYMENT_REJECTED + retry)
- [ ] Atualizar `docs/architecture.md` raiz (sequence diagram do pagamento)
- [ ] Atualizar `docs/business-rules.md` raiz (stateDiagram com PAYMENT_REJECTED)

**Total estimado**: 35 horas de implementação efetiva (vs. 22h sem Cancel/Refund + customer snapshot).

---

## 10. Riscos e Mitigações

| Risco | Probabilidade | Impacto | Mitigação |
|-------|---------------|---------|-----------|
| SDK não permite override de base URL | **Confirmado** | Alto (mock BDD) | Implementar Requester custom (Fase 3) |
| Orders API tem comportamento diferente de Preferences para Checkout Pro online | Médio | Alto | Validar em sandbox MP antes de mexer no mock; verificar Postman collection |
| Webhook payload da Orders API quebra parser atual | Alta | Alto | Atualizar handler (Fase 6) e teste de assinatura inválida |
| OpenTelemetry tracing perdido nas chamadas MP | Baixa | Médio | Injetar spans no Requester custom; documentar trade-off |
| Migration 1-step gera downtime no deploy | Média | Médio | Janela curta (< 1min); deploy atômico via Helm; rollback plan documentado |
| Falha no MP Cancel/Refund deixa OS em estado inconsistente | Média | Alto | Bloquear transição para CANCELED; reprocessar via DLQ; alerta operacional |
| Snapshot customer fica desatualizado (cliente trocou email/CPF) | Baixa | Médio | Aceitável — dados snapshot são do momento da criação da OS, que é o contrato fiscal |
| Customer não tem CPF cadastrado (PJ) | Média | Alto | Usar CNPJ no `Payer.Identification.Type = "CNPJ"`; já suportado pelo SDK |
| Endpoint `GET /payments/result` precisa de autenticação? | Baixa | Baixo | NÃO — é destino de redirect do MP (usuário final). Validar `order_id` na query e checar ownership opcional |
| BDD existente deixa de cobrir caminho real do SDK | Baixa | Médio | Manter o mock HTTP-level (não trocar por mock Go) |
| Retry de pagamento gera múltiplos `mp_order_id` órfãos no MP | Baixa | Baixo | Cancelar order anterior automaticamente antes de criar novo no retry |

---

## 11. Decisões Tomadas (consolidação)

| # | Pergunta original | Decisão | Implicação no plano |
|---|-------------------|---------|---------------------|
| 1 | Migration: 1 step ou 3 steps? | **1 step** com janela curta | `RENAME COLUMN` + ADD COLUMN num único `BEGIN/COMMIT` (seção 5) |
| 2 | Comportamento em pagamento rejeitado | **Novo status `PAYMENT_REJECTED`** | Adicionado à máquina de estados; permite retry ou cancelamento (seções 1.2, 5.1) |
| 3 | URLs callback/success/failure: nossas ou MP? | **Nossas** (MS2 serve HTML) | Novo handler `GET /payments/result`; nova var `MP_CALLBACK_BASE_URL` (seção 1.3) |
| 4 | CPF do customer: snapshot ou REST sync? | **Snapshot na OS** (mais performático) | Migration adiciona `customer_email`, `customer_cpf`, `customer_name`; `create_service_order` popula via REST sync único (seção 5) |
| 5 | Cancel/Refund agora ou depois? | **Agora** | Fase 4 adiciona usecases; Fase 7 integra ao saga de cancelamento de OS (seção 9) |

---

## 12. Plano de Rollback

Em caso de falha crítica após deploy:

1. **Backend**: redeploy da imagem anterior (tag pré-migration)
2. **Schema**: aplicar `003_rollback.sql`:
   ```sql
   BEGIN;
   ALTER TABLE service_orders RENAME COLUMN mp_order_id TO mp_preference_id;
   ALTER TABLE service_orders DROP COLUMN mp_order_status;
   ALTER TABLE service_orders DROP COLUMN payment_rejection_reason;
   -- Manter colunas customer_* (não causam impacto na versão antiga)
   COMMIT;
   ```
3. **OS em PAYMENT_REJECTED no momento do rollback**: forçar manualmente para `AWAITING_PAYMENT` via script de migração reversa
4. **mp_order_id criados no MP**: cancelar via script batch chamando MP Cancel (orders criados não pagos)

---

## 13. Referências

- SDK Go: https://github.com/mercadopago/sdk-go
- Orders API (PT): https://www.mercadopago.com.br/developers/pt/reference/order/online-payments/create/post
- Postman collection oficial: `D:\Estudos\GoLang\PosTech-OficinaTech\mercado-pagopostman_collection`
- Código atual: `internal/modules/billing/` no MS2

---

## 14. Diagrama do Fluxo Final

```mermaid
sequenceDiagram
    participant C as Cliente
    participant MS2 as ms-order-service
    participant SDK as SDK MP (Orders)
    participant MP as Mercado Pago
    participant MS3 as ms-workshop

    Note over C,MS3: Criação de OS (com snapshot do customer)
    C->>MS2: POST /service-orders
    MS2->>MS2: GET customer (REST sync único com MS1)
    MS2->>MS2: persiste customer_email, customer_cpf, customer_name

    Note over C,MP: Avanço para AWAITING_PAYMENT
    C->>MS2: POST /service-orders/{id}/advance (OS COMPLETED)
    MS2->>SDK: order.Create(items, payer com snapshot, urls)
    SDK->>MP: POST /v1/orders
    MP-->>SDK: {id, payments[0].payment_method.redirect_url}
    SDK-->>MS2: Order Response
    MS2->>MS2: persiste mp_order_id, payment_url, status=AWAITING_PAYMENT
    MS2-->>C: 202 {payment_url}

    Note over C,MP: Cliente paga (ou rejeitado)
    C->>MP: Checkout no payment_url

    alt Pagamento aprovado
        MP->>MS2: POST /payments/mp-webhook {data.id=order_id}
        MS2->>MS2: valida x-signature
        MS2->>SDK: order.Get(order_id)
        SDK-->>MS2: status=processed, payment=approved
        MS2->>MS2: OS → PAID, persiste mp_payment_id
    else Pagamento rejeitado
        MP->>MS2: webhook com data.id
        MS2->>SDK: order.Get(order_id)
        SDK-->>MS2: payment=rejected, status_detail=cc_rejected_*
        MS2->>MS2: OS → PAYMENT_REJECTED, salva motivo
        MS2->>C: email com URL de retry
    end

    Note over C,MS3: Cenário de Cancelamento (Cancel/Refund)
    alt Cancelar antes de pagar (AWAITING_PAYMENT)
        C->>MS2: POST /service-orders/{id}/cancel
        MS2->>SDK: order.Cancel(order_id)
        SDK->>MP: POST /v1/orders/{id}/cancel
        MS2->>MS3: SQS CANCEL_RESERVED (libera estoque)
        MS2->>MS2: OS → CANCELED
    else Cancelar após pagamento (PAID)
        C->>MS2: POST /service-orders/{id}/cancel
        MS2->>SDK: order.Refund(order_id, nil)
        SDK->>MP: POST /v1/orders/{id}/refund (total)
        alt Refund OK
            MS2->>MS3: SQS CANCEL_CONFIRMED (libera estoque)
            MS2->>MS2: OS → CANCELED (com nota de refund)
        else Refund falhou
            MS2->>MS2: bloqueia transição; OS permanece em PAID; alerta operacional
        end
    end
```

---

## 12. Referências

- SDK Go: https://github.com/mercadopago/sdk-go
- Orders API (PT): https://www.mercadopago.com.br/developers/pt/reference/order/online-payments/create/post
- Postman collection oficial: `D:\Estudos\GoLang\PosTech-OficinaTech\mercado-pagopostman_collection`
- Código atual: `internal/modules/billing/` no MS2
