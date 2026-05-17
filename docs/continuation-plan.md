# Plano de Continuação — Migração Mercado Pago Orders API

> **Branch ativa**: `feat/mercado-pago`
> **Último commit**: fase 8 — mock MP atualizado para Orders API
> **Data**: 2026-05-17

---

## Estado Atual (Fases 1–8 concluídas)

| Fase | Status | Commit | O que foi feito |
|------|--------|--------|-----------------|
| 1 — Setup | ✅ | `8127924` | SDK v1.8.1 no go.mod, migration 003, rollback script, .env.example |
| 2 — Domain | ✅ | `d2d718b` | Order/OrderItem/PayerInfo, StatusPaymentRejected, interface 5 métodos |
| 3 — Adapter SDK | ✅ | `67fe3cc` | sdk_client.go, requester.go, noop_client.go, remove HTTP client antigo |
| 4 — Application | ✅ | `fd986f4` | handle_payment_webhook usa GetOrder, retry_payment, cancel_payment_order, refund_payment_order |
| 5 — Customer Snapshot | ✅ | `a830a0f` | SetCustomerSnapshot, GORM model atualizado, CreateServiceOrder popula snapshot |
| 6 — HTTP | ✅ | `ec16b65` | ResultHandler (GET /payments/result), RetryPayment (POST /service-orders/{id}/retry-payment) |
| 7 — Saga Cancel/Refund | ✅ | `ee96776` | DeleteServiceOrder integra CancelPaymentOrder/RefundPaymentOrder antes do saga |
| 8 — Mock BDD | ✅ | `2898ebe` | mockmp reescrito para Orders API (/v1/orders/*), webhook com HMAC |
| **9 — BDD** | ⏳ | — | Pendente |
| **10 — Docs** | ⏳ | — | Pendente |

---

## Fase 9 — BDD (payment_steps.go + feature file)

### Contexto crítico

`lookupPreferenceID` em `bdd/steps/payment_steps.go` chama `fetchOrderRaw` (`GET /service-orders/{id}`)
que **não** inclui `mp_preference_id` no response. A função sempre vai timeout.

**Fix**: trocar para chamar `GET /service-orders/{id}/payment` que retorna `{mp_order_id, payment_url, status}`.

### O que mudar em `bdd/steps/payment_steps.go`

1. **Renomear `lookupPreferenceID` → `lookupOrderID`**
   - Chamar `GET /service-orders/{id}/payment` (usa `PaymentHandler.GetServiceOrderPayment`)
   - Buscar `mp_order_id` na resposta (não mais `mp_preference_id`)

2. **`triggerPaymentWebhook`** (approved):
   - `GET /service-orders/{id}/payment` → pegar `mp_order_id`
   - `POST /__mock/orders/{orderID}` com `{"payment_status": "approved"}`
   - `POST /payments/mp-webhook?data.id={orderID}` com HMAC assinado com `order_id`

3. **`triggerRejectedWebhook`** (rejected):
   - Mesmo fluxo mas `{"payment_status": "rejected"}`
   - Webhook vai para PAYMENT_REJECTED (não mais AWAITING_PAYMENT)

4. **`triggerInvalidSignatureWebhook`**:
   - Usar `order_id` no lugar de `paymentID` nos campos `data.id` e manifest HMAC

5. **Novos steps**:
   - `retryPayment(ctx)` → `POST /service-orders/{id}/retry-payment` → assert AWAITING_PAYMENT
   - `triggerCancelWithPaymentRefund(ctx)` → cancel OS em PAID → assert refund chamado + CANCELED

6. **Registrar novos steps em `RegisterPaymentSteps`**

### O que mudar em `features/payment_flow.feature`

```gherkin
# Cenário 1: approved → sem mudança de fluxo, só texto dos steps
Quando advance cria Order MP (mock retorna mp_order_id)
Então a OS está em AWAITING_PAYMENT
Quando MP webhook chega com status=approved
Então a OS está em PAID

# Cenário 2: rejected → PAYMENT_REJECTED (NÃO mais AWAITING_PAYMENT)
Quando MP webhook chega com status=rejected
Então a OS está em PAYMENT_REJECTED   # ← mudança!

# Cenário 3: assinatura inválida → sem mudança semântica

# Cenário 4 (novo): retry após rejeição
Dado uma OS em PAYMENT_REJECTED
Quando retry-payment é chamado
Então a OS está em AWAITING_PAYMENT
E um novo mp_order_id foi criado

# Cenário 5 (novo): cancelar OS em AWAITING_PAYMENT cancela Order no MP
Dado uma OS em AWAITING_PAYMENT
Quando cancel é chamado
Então o mock recebeu POST /v1/orders/{id}/cancel
E a OS está em CANCELED

# Cenário 6 (novo): cancelar OS em PAID faz refund no MP
Dado uma OS em PAID
Quando cancel é chamado
Então o mock recebeu POST /v1/orders/{id}/refund
E a OS está em CANCELED
```

### Arquivos a modificar na Fase 9

| Arquivo | Ação |
|---------|------|
| `bdd/steps/payment_steps.go` | Reescrever (lookupOrderID, novos steps, HMAC com order_id) |
| `features/payment_flow.feature` | Atualizar textos + adicionar 3 novos cenários |

---

## Fase 10 — Docs

### Arquivos a atualizar

| Arquivo | O que atualizar |
|---------|-----------------|
| `README.md` (MS2) | Seções: Mercado Pago, novos endpoints, PAYMENT_REJECTED, retry-payment, GET /payments/result |
| `docs/architecture.md` (MS2) | Fluxo MP: Preferences → Orders API; novos endpoints HTTP |
| `docs/business-rules.md` (MS2) | StatusPaymentRejected + regras de retry/cancel/refund |
| `docs/architecture.md` (root) | Sequence diagram de pagamento atualizado (Orders API) |
| `docs/business-rules.md` (root) | stateDiagram com PAYMENT_REJECTED adicionado |

### Comandos para actualizar business-rules.md raiz (stateDiagram)

Adicionar ao diagrama:
```
AWAITING_PAYMENT --> PAYMENT_REJECTED : webhook rejected
PAYMENT_REJECTED --> AWAITING_PAYMENT : retry-payment
PAYMENT_REJECTED --> CANCELED : cancel (CANCEL_CONFIRMED)
```

---

## Fases Opcionais (fora do escopo atual)

| Fase | Descrição |
|------|-----------|
| CPF no snapshot | Adicionar `customer_cpf` ao `CustomerDTO` e MS1 REST endpoint; popular `PayerInfo.CPF` |
| MPOrderStatus persistido | Atualizar `handle_payment_webhook.go` para salvar `MPOrderStatus` na OS |
| PaymentRejectionReason persistido | Salvar `PaymentStatusDetail` na coluna `payment_rejection_reason` |
| Testes unitários Cancel/Refund | Adicionar testes para `CancelPaymentOrder`, `RefundPaymentOrder`, `RetryPayment` usecases |
| Teste do ResultHandler | Adicionar test para `GET /payments/result?status=success|pending|failure` |

---

## Como retomar no próximo chat

1. Abrir o repo em `D:\Estudos\GoLang\PosTech-OficinaTech\oficina-tech-ms-order-service`
2. Checar branch: `git status` → branch `feat/mercado-pago`
3. Checar estado dos testes: `go test ./... -count=1`
4. Ler este arquivo + `docs/mercado-pago-migration.md` para contexto completo
5. Iniciar pela Fase 9: atualizar `bdd/steps/payment_steps.go`

### Contexto para passar ao próximo chat

```
Estou trabalhando no repositório oficina-tech-ms-order-service na branch feat/mercado-pago.
Fases 1-8 de uma migração do Mercado Pago (Preferences API → Orders API com SDK Go oficial) 
foram concluídas. Preciso implementar as Fases 9 e 10.

Leia docs/continuation-plan.md para o plano completo.
Leia docs/mercado-pago-migration.md para o plano de migração original.

Fase 9 começa por: bdd/steps/payment_steps.go e features/payment_flow.feature
Fase 10: docs/README.md, docs/architecture.md, docs/business-rules.md do MS2 e da raiz.
```

---

## Resumo de Arquivos Chave

```
internal/modules/billing/
  domain/payment/payment.go          ← Order struct (5 campos), MercadoPagoClient (5 métodos)
  application/usecases/
    create_payment_preference.go      ← cria Order MP (buildPayerInfo usa snapshot)
    handle_payment_webhook.go         ← GetOrder, PAYMENT_REJECTED, markApproved/Rejected
    retry_payment.go                  ← novo: PAYMENT_REJECTED → AWAITING_PAYMENT
    cancel_payment_order.go           ← novo: cancela Order MP antes de CANCELED
    refund_payment_order.go           ← novo: estorno antes de CANCELED pós-PAID
  infra/mercado_pago/
    sdk_client.go                     ← SDKClient, NewSDKClientFromEnv, buildSDKPayer
    requester.go                      ← RewritingRequester para BDD (MP_BASE_URL)
    noop_client.go                    ← 5 métodos mock para dev sem token
    signature_validator.go            ← HMAC-SHA256 (mantido, usa order_id no manifest)
  infra/http/
    handlers/result_handler.go        ← novo: GET /payments/result HTML
    handlers/payment_handler.go       ← GetServiceOrderPayment + RetryPayment
    handlers/webhook_handler.go       ← MPOrderID (era PaymentID)
    routes.go                         ← 4 rotas billing

internal/modules/service_order/
  domain/service_order/
    order_status.go                   ← StatusPaymentRejected + transições
    service_order.go                  ← SetCustomerSnapshot, CustomerEmail, CustomerName
  application/
    usecases/delete_service_order.go  ← injeta Cancel/RefundPaymentOrder
    saga/saga_orchestrator.go         ← CancelOrder inclui StatusPaymentRejected
  infra/persistence/service_order_model.go ← MPOrderID (era MPPreferenceID), colunas novas

bdd/
  mockmp/main.go                      ← Orders API: /v1/orders/*, /__mock/orders/*
  steps/payment_steps.go              ← PENDENTE (Fase 9)
  features/payment_flow.feature       ← PENDENTE (Fase 9)

migrations/
  003_mp_orders_api_migration.sql     ← rename + 5 novas colunas
  003_mp_orders_api_rollback.sql      ← rollback script
```
