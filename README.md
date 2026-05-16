# oficina-tech-ms-order-service

**MS2 — OS Service (Saga Orchestrator)**

Microsserviço central da plataforma. Gerencia o ciclo de vida completo das ordens de serviço, **orquestra o Saga Pattern** de controle de estoque com o MS3, integra com o Mercado Pago para pagamentos e notifica clientes por email a cada transição de status.

---

## Arquitetura Interna

```
cmd/api/main.go
    │
    ├── internal/modules/
    │   ├── service_order/      ← OS, saga orchestrator, máquina de estados
    │   ├── billing/            ← integração Mercado Pago (AWAITING_PAYMENT → PAID)
    │   └── access_control/     ← validação JWT local (sem chamada ao MS1)
    │
    ├── internal/messaging/
    │   ├── consumers/
    │   │   ├── customer_deleted_consumer.go      ← MS1 → MS2
    │   │   └── order_inventory_event_consumer.go ← MS3 → MS2
    │   └── publishers/
    │       └── order_inventory_op_publisher.go   ← MS2 → MS3
    │
    ├── internal/shared/
    │   ├── dto/                ← DTOs compartilhados
    │   ├── infra/http_clients/ ← clientes REST para MS1 e MS3
    │   ├── infra/database/     ← PostgreSQL (GORM) + DynamoDB
    │   ├── infra/http/         ← middleware JWT e RBAC
    │   └── infra/observability/← OpenTelemetry
    │
    ├── bdd/                    ← runner Godog + step definitions
    ├── features/               ← arquivos Gherkin (.feature)
    ├── migrations/             ← SQL schema
    ├── k8s/                    ← manifests Kubernetes
    └── infra/terraform/        ← IaC do serviço
```

### Camadas por módulo

```
handler (HTTP) → usecase → saga_orchestrator → repository (PG) + history (DynamoDB)
                               │
                               ├── adapter REST → MS1 (customers, vehicles)
                               ├── adapter REST → MS3 (products, services — snapshot)
                               └── messaging publisher → SQS (saga steps)

messaging/consumers → usecase (avança estado da OS ou dispara compensação)
```

---

## Módulos Internos

### service_order

O módulo central — implementa a máquina de estados da OS e orquestra o Saga.

**Use cases:**

| Use Case | Descrição |
|----------|-----------|
| `create` | Valida cliente/veículo (REST MS1), captura snapshots de preço (REST MS3), cria OS com status RECEIVED |
| `advance_status` | Avança a máquina de estados; dispara saga quando necessário |
| `respond_to_authorization` | Aprovação/rejeição pelo cliente (PENDING_AUTHORIZATION) |
| `delete` | Cancela OS; dispara compensação de estoque se necessário |
| `get` / `get_all` | Consulta OS (com filtros por status e customer_id) |
| `get_history` | Retorna histórico de status do DynamoDB |

**Adapters (HTTP clients para outros serviços):**

| Adapter | Destino | Endpoint |
|---------|---------|----------|
| `customer_adapter` | MS1 | `GET /customers/{id}` |
| `vehicle_adapter` | MS1 | `GET /vehicles/{id}` |
| `product_adapter` | MS3 | `GET /products/{id}` |
| `service_adapter` | MS3 | `GET /services/{id}` |

**Saga Orchestrator (`application/saga/saga_orchestrator.go`):**

Controla o `saga_status` de cada OS (`IDLE | AWAITING_INVENTORY | AWAITING_PAYMENT | FAILED`). A cada evento recebido do MS3 ou do Mercado Pago, decide o próximo estado da OS.

### billing

Integração com Mercado Pago para o fluxo de pagamento.

| Use Case | Descrição |
|----------|-----------|
| `create_payment_preference` | Cria preferência no MP na transição COMPLETED → AWAITING_PAYMENT |
| `get_payment_status` | Consulta status de um pagamento (`GET /v1/payments/{id}`) |
| `handle_payment_webhook` | Processa notificação do MP; valida assinatura (`x-signature + MP_WEBHOOK_SECRET`) |

`infra/mercado_pago/` — client HTTP, DTOs e `signature_validator`.

Na inicialização, OS com `saga_status = AWAITING_PAYMENT` apenas são logadas — o MS2 não republica mensagens SQS para elas porque o webhook do Mercado Pago retomará o fluxo quando o cliente pagar.

### access_control

Validação local de JWT. Não realiza chamadas ao MS1 — usa o `JWT_SECRET_KEY` compartilhado para verificar assinatura e expiração do token.

---

## Máquina de Estados da OS

```
RECEIVED
    │ advance (sem saga)
    ▼
DIAGNOSING
    │ advance → SAGA: publica RESERVE, saga_status = AWAITING_INVENTORY
    │           aguarda order-inventory-op-succeeded/failed
    ▼
PENDING_AUTHORIZATION
    │ authorize(approved=true)      │ authorize(approved=false)
    │ (sem saga)                    │ SAGA: publica CANCEL_RESERVED
    ▼                               ▼
AUTHORIZED               AUTHORIZATION_DENIED (FINAL)
    │ advance (sem saga)
    ▼
IN_PROGRESS
    │ advance → SAGA: publica RESERVED_DECREASE, saga_status = AWAITING_INVENTORY
    │           aguarda order-inventory-op-succeeded/failed
    ▼
COMPLETED
    │ advance → cria preferência MP, saga_status = AWAITING_PAYMENT
    ▼
AWAITING_PAYMENT
    │ webhook MP approved
    ▼
PAID
    │ advance (sem saga)
    ▼
DELIVERED (FINAL)

──────────────────────────────────────────────────────────────
CANCELED (FINAL) — acessível de qualquer estado não-final
  Antes de COMPLETED  → SAGA: publica CANCEL_RESERVED
  Após COMPLETED      → SAGA: publica CANCEL_CONFIRMED
```

### Operações de estoque por transição

| De | Para | Operação no MS3 | Tipo Saga |
|---|---|---|---|
| DIAGNOSING | PENDING_AUTHORIZATION | RESERVE | Forward step |
| IN_PROGRESS | COMPLETED | RESERVED_DECREASE | Forward step |
| PENDING_AUTH | AUTHORIZATION_DENIED | CANCEL_RESERVED | Compensação |
| Qualquer¹ | CANCELED | CANCEL_RESERVED | Compensação |
| COMPLETED+ | CANCELED | CANCEL_CONFIRMED | Compensação |

¹ RECEIVED, DIAGNOSING, PENDING_AUTH, AUTHORIZED, IN_PROGRESS

---

## Banco de Dados

**Porta local**: PostgreSQL `5434`, DynamoDB via LocalStack

### PostgreSQL — `db_ms2`

```
service_orders (
  id UUID PK, customer_id, vehicle_id,
  status,          -- RECEIVED | DIAGNOSING | PENDING_AUTHORIZATION | AUTHORIZED |
                   -- IN_PROGRESS | COMPLETED | AWAITING_PAYMENT | PAID | DELIVERED |
                   -- AUTHORIZATION_DENIED | CANCELED
  total_amount,
  saga_status,     -- IDLE | AWAITING_INVENTORY | AWAITING_PAYMENT | FAILED
  saga_id,         -- UUID da execução corrente do saga (idempotência)
  mp_preference_id,    -- ID da preferência Mercado Pago
  mp_payment_id,       -- ID do pagamento confirmado
  notes, closed_at,
  created_at, updated_at, deleted_at
)

service_order_items (
  id UUID PK, service_order_id FK,
  item_type,      -- SERVICE | PRODUCT
  reference_id,   -- ID no MS3 (snapshot — não consultado novamente)
  name,           -- snapshot do nome no momento da criação
  unit_price,     -- snapshot do preço no momento da criação
  quantity, subtotal,
  created_at, updated_at, deleted_at
)
```

> Items são imutáveis a partir de PENDING_AUTHORIZATION — o estoque já foi reservado com base neles.

### DynamoDB — tabela `order_history`

```json
{
  "order_id": "uuid",          // partition key
  "occurred_at": "ISODate",   // sort key
  "previous_status": "DIAGNOSING",
  "new_status": "PENDING_AUTHORIZATION",
  "changed_by": "user_id",
  "notes": "string",
  "snapshot": {
    "items": [
      { "item_type": "PRODUCT", "reference_id": "uuid", "name": "Filtro de óleo", "quantity": 2, "unit_price": 4500 }
    ],
    "total_amount": 24000,
    "customer_id": "uuid",
    "vehicle_id": "uuid"
  }
}
```

Schema gerenciado via migrations SQL em `migrations/`.

---

## Endpoints HTTP

**Porta**: `8082`

```
POST   /service-orders                   Criar OS (status inicial: RECEIVED)
GET    /service-orders                   Listar OS (filtros: status, customer_id)
GET    /service-orders/{id}             Buscar OS por ID
PUT    /service-orders/{id}             Atualizar itens (só em RECEIVED ou DIAGNOSING)
DELETE /service-orders/{id}             Cancelar OS (dispara saga de compensação)

POST   /service-orders/{id}/advance     Avançar status (MECHANIC/ADMIN)
POST   /service-orders/{id}/authorize   Aprovar ou rejeitar OS (CUSTOMER/ADMIN)
GET    /service-orders/{id}/history     Histórico de status (DynamoDB)
GET    /service-orders/{id}/payment     Retorna payment_url do Mercado Pago

POST   /payments/mp-webhook             Recebe notificações do Mercado Pago
```

---

## Eventos

### Publicados (SQS — MS2 → MS3)

```json
// Fila: order-inventory-op-requested
{
  "event": "OrderInventoryOpRequested",
  "saga_id": "uuid",
  "order_id": "uuid",
  "operation": "RESERVE",  // RESERVE | RESERVED_DECREASE | CANCEL_RESERVED | CANCEL_CONFIRMED
  "items": [{ "product_id": "uuid", "quantity": 2 }],
  "occurred_at": "RFC3339"
}
```

### Consumidos (SQS)

```
order-inventory-op-succeeded  (MS3 → MS2)  Confirma saga step → MS2 avança status da OS
order-inventory-op-failed     (MS3 → MS2)  Falha no saga step → MS2 registra e notifica
customer-deleted              (MS1 → MS2)  Cancela todas as OS ativas do customer_id
```

### Idempotência e Recuperação

O MS2 verifica o `saga_id` antes de processar qualquer evento:
- Se `saga_status != AWAITING_INVENTORY` ou `saga_id` não corresponde → descarta (duplicata)
- Se o MS2 reiniciar com `saga_status = AWAITING_INVENTORY` → republica o evento com o mesmo `saga_id`
- Se reiniciar com `saga_status = AWAITING_PAYMENT` → apenas loga; o webhook do MP retomará o fluxo

---

## BDD — Godog

Testes de comportamento cobrindo os fluxos do Saga em `features/` (Gherkin em português).

| Feature | Cenários cobertos |
|---------|------------------|
| `service_order_lifecycle.feature` | Ciclo completo RECEIVED → DELIVERED |
| `saga_compensation.feature` | Estoque insuficiente, autorização negada, cancelamento |
| `payment_flow.feature` | COMPLETED → AWAITING_PAYMENT → PAID via webhook MP |
| `customer_deleted.feature` | Cancelamento automático de OS ao deletar cliente |
| `saga_recovery.feature` | Recuperação após reinício com saga em andamento |

```bash
# Sobe ambiente E2E (todos os serviços + LocalStack + mock Mercado Pago)
docker-compose -f docker-compose.e2e.yml up -d

# Roda BDD
cd bdd && go test -v ./...
```

Variáveis necessárias para o BDD: `MS1_URL`, `MS2_URL`, `MS3_URL`, `MP_MOCK_URL`, `MP_WEBHOOK_SECRET`.

---

## Variáveis de Ambiente

```env
SERVER_PORT=8082

DB_HOST=localhost
DB_PORT=5434
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=db_ms2

AWS_REGION=us-east-1
AWS_ENDPOINT=http://localhost:4566          # LocalStack em dev
DYNAMODB_TABLE_ORDER_HISTORY=order_history
SQS_INVENTORY_OP_REQUESTED_URL=            # MS2 publica (→ MS3)
SQS_INVENTORY_OP_RESULT_URL=               # MS2 consome (← MS3)
SQS_CUSTOMER_DELETED_URL=                  # MS2 consome (← MS1)

JWT_SECRET_KEY=                            # compartilhado com todos os serviços

MS1_BASE_URL=http://ms-identity:8081
MS3_BASE_URL=http://ms-workshop:8083

MP_ACCESS_TOKEN=                           # Mercado Pago (Secrets Manager)
MP_WEBHOOK_SECRET=                         # validação de assinatura do webhook
MP_NOTIFICATION_URL=                       # URL pública do webhook

SMTP_HOST=
SMTP_PORT=
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=

OTEL_EXPORTER_OTLP_ENDPOINT=              # opcional — observabilidade
```

---

## Como Rodar Localmente

```bash
# Sobe PostgreSQL + LocalStack (SQS + DynamoDB)
docker-compose up -d

# Instala dependências
go mod download

# Roda o serviço
go run cmd/api/main.go
```

A API fica disponível em `http://localhost:8082`.

---

## Testes

```bash
# Unitários — foco na máquina de estados e saga orchestrator
go test ./internal/modules/service_order/application/...

# BDD — fluxo completo e compensações
cd bdd && go test -v ./...

# Cobertura mínima: 80%
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## Infraestrutura

```
k8s/
├── deployment.yaml         2 réplicas, probes, env from ConfigMap/Secret
├── service.yaml            ClusterIP → NodePort 30082
├── hpa.yaml
└── secret.yaml.example     JWT, DB, MP, SMTP credentials

infra/terraform/            IaC do serviço
migrations/                 SQL schema (service_orders, service_order_items)
```

Pipeline CI/CD: `.github/workflows/` — `ci.yml` → `deploy.yml` (inclui BDD) → `release.yml` | `rollback.yml`
