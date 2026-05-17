# Regras de Negócio — ms-order-service

> Regras globais de RBAC e ciclo de vida: [docs/business-rules.md](../../docs/business-rules.md)
> Arquitetura interna e endpoints: [docs/architecture.md](./architecture.md)

---

## Máquina de Estados da OS

12 estados definidos em `internal/modules/service_order/domain/service_order/order_status.go`.

```
RECEIVED → DIAGNOSING → PENDING_AUTHORIZATION → AUTHORIZED → IN_PROGRESS → COMPLETED
                                                                              │
                                                                    AWAITING_PAYMENT ─┬─→ PAID → DELIVERED
                                                                              │        └─→ PAYMENT_REJECTED
                                                                              │                   │
                                                                              │    retry-payment ◄─┘
                                                                              │
                                                               (cancelamento) └─→ CANCELED
```

### Transições válidas

| De | Para | Gatilho |
|----|------|---------|
| `RECEIVED` | `DIAGNOSING` | advance |
| `DIAGNOSING` | `PENDING_AUTHORIZATION` | saga RESERVE OK |
| `PENDING_AUTHORIZATION` | `AUTHORIZED` | cliente aprova |
| `PENDING_AUTHORIZATION` | `AUTHORIZATION_DENIED` | cliente recusa |
| `AUTHORIZED` | `IN_PROGRESS` | advance |
| `IN_PROGRESS` | `COMPLETED` | saga RESERVED_DECREASE OK |
| `COMPLETED` | `AWAITING_PAYMENT` | Order MP criado com sucesso |
| `AWAITING_PAYMENT` | `PAID` | webhook MP `payment.status = approved` |
| `AWAITING_PAYMENT` | `PAYMENT_REJECTED` | webhook MP `payment.status = rejected` |
| `PAYMENT_REJECTED` | `AWAITING_PAYMENT` | `POST /service-orders/{id}/retry-payment` |
| `PAID` | `DELIVERED` | advance |
| Qualquer não-terminal | `CANCELED` | DELETE (com compensação) |

**Terminais**: `DELIVERED`, `CANCELED`, `AUTHORIZATION_DENIED`.

---

## Status `PAYMENT_REJECTED`

`PAYMENT_REJECTED` **não é terminal** — permite duas saídas:

1. **Retry**: `POST /service-orders/{id}/retry-payment`
   - Cancela o order anterior no Mercado Pago (evita cobrança futura)
   - Cria um novo Order MP com novo `mp_order_id`
   - OS volta para `AWAITING_PAYMENT`
   - Não há limite de retentativas (política de throttle a ser definida em produção)

2. **Cancelamento explícito**: `DELETE /service-orders/{id}`
   - Chama `CancelPaymentOrder` antes de transicionar para `CANCELED`
   - Falha no MP Cancel não bloqueia o cancelamento (idempotente — 404/410 são aceitos)
   - Dispara `CANCEL_CONFIRMED` no saga de estoque

### Email de rejeição

Quando o webhook retorna `rejected`, um email é enviado ao cliente com:
- Motivo de rejeição (`payment_rejection_reason` = `status_detail` do MP)
- URL para retry: `POST /service-orders/{id}/retry-payment`

---

## Cancelamento e Estorno

O comportamento de cancelamento depende do status atual da OS:

| Status atual | Ação no Mercado Pago | Saga de estoque | Bloqueante se MP falhar? |
|-------------|----------------------|-----------------|--------------------------|
| RECEIVED, DIAGNOSING, AUTHORIZED, PENDING_AUTHORIZATION, IN_PROGRESS | — | `CANCEL_RESERVED` | Não |
| COMPLETED, AWAITING_PAYMENT, PAYMENT_REJECTED | `CancelOrder` (404/410 aceitos) | `CANCEL_CONFIRMED` | Não |
| PAID | `RefundOrder` (estorno total) | `CANCEL_CONFIRMED` | **Sim** |

**Regra crítica**: falha no `RefundOrder` (ex: MP retorna 5xx ou pedido já estornado com erro) **bloqueia** a transição para `CANCELED` — a OS permanece em `PAID`. O operacional deve investigar e reprocessar manualmente (via DLQ ou script batch).

---

## Snapshot do Customer

Na criação da OS, o MS2 faz REST sync com o MS1 (`GET /customers/{id}`) e persiste `customer_email` e `customer_name` na tabela `service_orders`. Esses campos são usados pelo SDK MP para preencher o `Payer` no momento do pagamento — nenhuma chamada adicional ao MS1 é feita no fluxo de pagamento.

**Implicação**: se o cliente atualizar email/nome após a criação da OS, o snapshot fica desatualizado. Isso é aceitável — os dados de snapshot representam o contrato no momento da criação.

---

## Idempotência do Webhook

O webhook handler verifica:
1. Assinatura HMAC-SHA256 válida (`x-signature: ts={ts},v1={hmac}`) com `MP_WEBHOOK_SECRET`
2. Manifest: `id:{order_id};request-id:{request-id};ts:{ts};`
3. `data.id` presente e não vazio

Se a OS já está em `PAID` e chega novo webhook `approved`, a transição é descartada silenciosamente (idempotência via `CanTransitionTo`). Qualquer falha na validação retorna 401 sem modificar o estado da OS.

---

## Recuperação de Sagas na Inicialização

`RecoverAwaitingSagas` percorre OS com `saga_status = AWAITING_INVENTORY` e republica o evento SQS.

OS com `saga_status = AWAITING_PAYMENT` **não** são republicadas — o webhook do Mercado Pago retoma o fluxo quando o cliente efetua o pagamento.

---

## Regras de Itens da OS

- Itens só podem ser modificados em estados editáveis: `RECEIVED`, `DIAGNOSING`, `PENDING_AUTHORIZATION`, `AUTHORIZED`
- A partir de `IN_PROGRESS`, itens são imutáveis (snapshot de preço já confirmado)
- OS sem itens avança localmente sem publicar no SQS (sem operação de estoque)
