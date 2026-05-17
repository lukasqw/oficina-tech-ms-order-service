# oficina-tech-ms-order-service

**MS2 — OS Service (Saga Orchestrator)**

Microsserviço central da plataforma Oficina Tech. Gerencia o ciclo de vida completo das ordens de serviço (OS), orquestra o Saga Pattern de controle de estoque com o MS3 via SQS, integra com o Mercado Pago para pagamentos e notifica clientes por email a cada transição de status.

---

## Responsabilidades

- Criar e gerenciar ordens de serviço com máquina de estados estrita (11 estados)
- Orquestrar operações de estoque no MS3 via Saga Pattern assíncrono (SQS)
- Integrar com Mercado Pago: criação de preferência de pagamento, processamento de webhook, validação de assinatura
- Validar cliente e veículo no MS1 via REST na criação da OS (snapshot)
- Capturar snapshot de preço de produtos e serviços do MS3 na criação da OS
- Cancelar automaticamente OS ativas ao receber evento `customer-deleted` do MS1
- Registrar histórico de transições de status no DynamoDB
- Recuperar sagas interrompidas ao reiniciar (`AWAITING_INVENTORY`)

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
    │   │   ├── customer_deleted.go               ← MS1 → MS2 (fila customer-deleted)
    │   │   └── order_inventory_op_failed.go      ← MS3 → MS2 (fila order-inventory-op-failed)
    │   │   └── order_inventory_op_succeeded.go   ← MS3 → MS2 (fila order-inventory-op-succeeded)
    │   └── publishers/
    │       └── order_inventory_op_publisher.go   ← MS2 → MS3 (fila order-inventory-op-requested)
    │
    ├── internal/shared/
    │   ├── dto/                    ← DTOs compartilhados entre módulos
    │   ├── http_clients/           ← clientes REST para MS1 (ms1_client.go) e MS3 (ms3_client.go)
    │   ├── infra/awsconfig/        ← configuração AWS SDK
    │   ├── infra/database/         ← PostgreSQL via GORM
    │   ├── infra/dynamodb/         ← cliente DynamoDB
    │   ├── infra/sqs/              ← cliente SQS + resolução de URLs de filas
    │   ├── infra/email/            ← serviço SMTP para notificações
    │   ├── infra/http/middleware/  ← middleware JWT e RBAC
    │   └── infra/observability/    ← OpenTelemetry (traces, métricas, logs)
    │
    ├── bdd/                    ← runner Godog + step definitions
    ├── features/               ← arquivos Gherkin (.feature)
    ├── migrations/             ← SQL schema (PostgreSQL)
    ├── k8s/                    ← manifestos Kubernetes
    └── infra/                  ← IaC Terraform do serviço
```

### Camadas por módulo

```
handler (HTTP) → usecase → saga_orchestrator → repository (PostgreSQL)
                                │                    + history_repository (DynamoDB)
                                ├── adapter REST → MS1 (customer_adapter, vehicle_adapter)
                                ├── adapter REST → MS3 (product_adapter, service_adapter)
                                └── publisher → SQS (order-inventory-op-requested)

messaging/consumers → saga_orchestrator.HandleSucceeded / HandleFailed / CancelOrder
```

---

## Módulos Internos

### service_order

Módulo central — implementa a máquina de estados e orquestra o Saga.

**Use cases:**

| Use Case | Descrição |
|----------|-----------|
| `CreateServiceOrder` | Valida cliente e veículo via REST no MS1; captura snapshots de preço via REST no MS3; persiste OS com status `RECEIVED` |
| `AdvanceServiceOrderStatus` | Avança o status seguindo a máquina de estados; dispara saga via SQS quando a transição exige operação de estoque |
| `RespondToAuthorization` | Processamento de aprovação (`approved=true`) ou rejeição (`approved=false`) pelo cliente em `PENDING_AUTHORIZATION` |
| `UpdateServiceOrder` | Atualiza dados da OS (itens, cliente, veículo); restrito a estados editáveis |
| `DeleteServiceOrder` | Soft delete com cancelamento da OS; dispara compensação de estoque via saga se necessário |
| `GetServiceOrder` | Busca OS por ID com detalhes completos |
| `GetAllServiceOrders` | Lista OS com filtros por status e `customer_id` |
| `GetServiceOrderHistory` | Retorna histórico de transições de status registrado no DynamoDB |

**Adapters REST (infra/adapters/implementations/):**

| Adapter | Destino | Endpoint |
|---------|---------|----------|
| `customer_adapter.go` | MS1 | `GET /customers/{id}` |
| `vehicle_adapter.go` | MS1 | `GET /vehicles/{id}` |
| `product_adapter.go` | MS3 | `GET /products/{id}` |
| `service_adapter.go` | MS3 | `GET /services/{id}` |

**Saga Orchestrator (`application/saga/saga_orchestrator.go`):**

Controla o `saga_status` de cada OS (`IDLE`, `AWAITING_INVENTORY`, `AWAITING_PAYMENT`, `FAILED`). Expõe os métodos:

- `StartSaga` — inicia uma operação de estoque assíncrona; se a OS não tem itens, faz a transição local sem SQS
- `CancelOrder` — determina a operação de compensação correta com base no status atual e inicia a saga
- `HandleSucceeded` — processa evento de sucesso do MS3; avança o status da OS
- `HandleFailed` — processa evento de falha do MS3; registra falha e notifica
- `RecoverAwaitingSagas` — chamado na inicialização; republica operações pendentes com `saga_status = AWAITING_INVENTORY`

### billing

Integração com Mercado Pago para o fluxo de pagamento.

**Use cases:**

| Use Case | Descrição |
|----------|-----------|
| `CreatePaymentPreference` | Cria preferência de pagamento no Mercado Pago na transição `COMPLETED → AWAITING_PAYMENT`; persiste `mp_preference_id` e `payment_url` na OS |
| `GetPaymentStatus` | Retorna `payment_url`, `mp_preference_id` e `status` de pagamento de uma OS |
| `HandlePaymentWebhook` | Processa notificação recebida do Mercado Pago; valida assinatura via `x-signature` + `MP_WEBHOOK_SECRET`; atualiza `mp_payment_id` e avança OS para `PAID` |

`infra/mercado_pago/` contém: `client.go` (HTTP client), `dtos.go`, `signature_validator.go` e `noop_client.go` (para testes).

### access_control

Validação local de JWT (HS256). Não realiza chamadas ao MS1 em tempo de execução — usa `JWT_SECRET_KEY` compartilhado para verificar assinatura e expiração do token.

---

## Máquina de Estados da OS

**Estados definidos em** `internal/modules/service_order/domain/service_order/order_status.go`:

```
RECEIVED, DIAGNOSING, PENDING_AUTHORIZATION, AUTHORIZED,
IN_PROGRESS, COMPLETED, AWAITING_PAYMENT, PAID, DELIVERED,
CANCELED, AUTHORIZATION_DENIED
```

**Diagrama de transições:**

```
RECEIVED
    │ advance (sem saga)
    ▼
DIAGNOSING
    │ advance → SAGA: publica RESERVE
    │           saga_status = AWAITING_INVENTORY
    │           aguarda order-inventory-op-succeeded/failed
    ▼
PENDING_AUTHORIZATION
    │ authorize(approved=true)          │ authorize(approved=false)
    │ (sem saga)                        │ SAGA: publica CANCEL_RESERVED
    ▼                                   ▼
AUTHORIZED                    AUTHORIZATION_DENIED (FINAL)
    │ advance (sem saga)
    ▼
IN_PROGRESS
    │ advance → SAGA: publica RESERVED_DECREASE
    │           saga_status = AWAITING_INVENTORY
    │           aguarda order-inventory-op-succeeded/failed
    ▼
COMPLETED
    │ advance → cria preferência Mercado Pago
    │           saga_status = AWAITING_PAYMENT
    ▼
AWAITING_PAYMENT
    │ webhook MP (payment aprovado)
    ▼
PAID
    │ advance (sem saga)
    ▼
DELIVERED (FINAL)

─────────────────────────────────────────────────────────────────
CANCELED (FINAL) — acessível de qualquer estado não-final
  De RECEIVED, DIAGNOSING, AUTHORIZED, PENDING_AUTHORIZATION,
    IN_PROGRESS               → SAGA: publica CANCEL_RESERVED
  De COMPLETED, AWAITING_PAYMENT, PAID  → SAGA: publica CANCEL_CONFIRMED
```

### Operações de estoque por transição

Definidas em `internal/modules/service_order/domain/service_order/inventory_operation_type.go`:

| De | Para | Operação SQS | Tipo |
|----|------|-------------|------|
| DIAGNOSING | PENDING_AUTHORIZATION | `RESERVE` | Forward step |
| IN_PROGRESS | COMPLETED | `RESERVED_DECREASE` | Forward step |
| PENDING_AUTHORIZATION | AUTHORIZATION_DENIED | `CANCEL_RESERVED` | Compensação |
| RECEIVED / DIAGNOSING / AUTHORIZED / PENDING_AUTHORIZATION / IN_PROGRESS | CANCELED | `CANCEL_RESERVED` | Compensação |
| COMPLETED / AWAITING_PAYMENT / PAID | CANCELED | `CANCEL_CONFIRMED` | Compensação |

---

## Endpoints HTTP

**Porta**: `8082`

| Método | Path | Role requerida | Descrição |
|--------|------|----------------|-----------|
| `POST` | `/service-orders` | USER, MANAGER, ADMIN | Criar OS (status inicial: `RECEIVED`) |
| `GET` | `/service-orders` | CUSTOMER, USER, MANAGER, ADMIN | Listar OS (filtros: status, customer_id) |
| `GET` | `/service-orders/{id}` | USER, MANAGER, ADMIN | Buscar OS por ID |
| `GET` | `/service-orders/{id}/history` | USER, MANAGER, ADMIN | Histórico de transições (DynamoDB) |
| `PUT` | `/service-orders/{id}` | USER, MANAGER, ADMIN | Atualizar OS (itens, cliente, veículo) |
| `POST` | `/service-orders/{id}/advance` | USER, MANAGER, ADMIN | Avançar status da OS |
| `POST` | `/service-orders/{id}/authorize` | CUSTOMER, USER, MANAGER, ADMIN | Aprovar ou rejeitar OS |
| `DELETE` | `/service-orders/{id}` | MANAGER, ADMIN | Cancelar OS (soft delete + compensação) |
| `GET` | `/service-orders/{id}/payment` | USER, MANAGER, ADMIN | Retorna `payment_url` e `mp_preference_id` |
| `POST` | `/payments/mp-webhook` | Público (validado por assinatura) | Recebe notificações do Mercado Pago |
| `GET` | `/health` | Público | Health check |

---

## Saga Pattern

O Saga é orquestrado pelo `Orchestrator` em `internal/modules/service_order/application/saga/saga_orchestrator.go`.

**Fluxo de um step de saga:**

1. O usecase chama `orchestrator.StartSaga(orderID, operation, targetStatus)`
2. O orchestrator gera um `saga_id` (UUID v7), persiste `saga_status = AWAITING_INVENTORY` na OS e publica a mensagem na fila `order-inventory-op-requested`
3. O MS3 processa a operação e publica resultado em `order-inventory-op-succeeded` ou `order-inventory-op-failed`
4. O consumer do MS2 chama `HandleSucceeded` ou `HandleFailed`
5. `HandleSucceeded` valida o `saga_id` para garantir idempotência; avança o status da OS para `targetStatus`; persiste histórico no DynamoDB; envia email de notificação ao cliente
6. `HandleFailed` registra a falha; mantém OS no status anterior

**Idempotência:** antes de processar qualquer evento de saga, o orchestrator verifica se `saga_status == AWAITING_INVENTORY` e se o `saga_id` do evento corresponde ao registrado na OS. Eventos duplicados ou fora de ordem são descartados silenciosamente.

**Recuperação na inicialização:** `RecoverAwaitingSagas` percorre todas as OS com `saga_status = AWAITING_INVENTORY` e republica o evento SQS com o mesmo `saga_id`. OS com `saga_status = AWAITING_PAYMENT` não são republicadas — o webhook do Mercado Pago retoma o fluxo.

**OS sem itens:** quando uma OS não possui itens, o orchestrator realiza a transição de status diretamente (sem publicar em SQS).

---

## Integração Mercado Pago

O fluxo de pagamento é iniciado pela transição `IN_PROGRESS → COMPLETED`:

1. `AdvanceServiceOrderStatus` avança a OS para `COMPLETED`
2. O saga step `RESERVED_DECREASE` é confirmado pelo MS3
3. O usecase `CreatePaymentPreference` cria uma preferência no Mercado Pago via `POST /checkout/preferences`
4. `mp_preference_id` e `payment_url` são persistidos na tabela `service_orders`
5. O status avança automaticamente para `AWAITING_PAYMENT`
6. O cliente acessa o link de pagamento via `GET /service-orders/{id}/payment`
7. Após o pagamento, o Mercado Pago envia `POST /payments/mp-webhook`
8. O `WebhookHandler` valida a assinatura `x-signature` com `MP_WEBHOOK_SECRET`
9. `HandlePaymentWebhook` consulta o pagamento no MP via `GET /v1/payments/{id}`, persiste `mp_payment_id` e avança a OS para `PAID`

---

## Eventos SQS

### Publicados (MS2 → MS3)

**Fila:** `order-inventory-op-requested`

```json
{
  "saga_id": "uuid-v7",
  "order_id": "uuid",
  "operation": "RESERVE",
  "items": [
    { "reference_id": "uuid", "item_type": "PRODUCT", "quantity": 2 }
  ],
  "occurred_at": "RFC3339"
}
```

Operações possíveis: `RESERVE`, `RESERVED_DECREASE`, `CANCEL_RESERVED`, `CANCEL_CONFIRMED`.

### Consumidos (MS3 → MS2)

| Fila | Publicador | Ação no MS2 |
|------|-----------|------------|
| `order-inventory-op-succeeded` | MS3 | Avança OS para `targetStatus`; registra histórico; envia email |
| `order-inventory-op-failed` | MS3 | Registra falha; mantém status anterior |

### Consumidos (MS1 → MS2)

| Fila | Publicador | Ação no MS2 |
|------|-----------|------------|
| `customer-deleted` | MS1 | Cancela todas as OS ativas do `customer_id`; dispara compensação de estoque para cada uma |

---

## Banco de Dados

### PostgreSQL — banco `db_ms2`

Porta local: `5434` (docker-compose.yml), `5432` (dentro do container).

Schema gerenciado pelas migrations em `migrations/`:

**`migrations/001_service_orders.sql`** — tabelas principais:

```sql
service_orders (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id       UUID NOT NULL,
  vehicle_id        UUID NOT NULL,
  description       TEXT,
  status            VARCHAR(50) NOT NULL,
  saga_status       VARCHAR(50) NOT NULL DEFAULT 'IDLE',
  current_saga_id   UUID NULL,
  saga_target_status VARCHAR(50) NULL,
  saga_notes        TEXT NULL,
  closed_at         TIMESTAMP,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at        TIMESTAMP
)

service_order_histories (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_order_id  UUID NOT NULL,
  metadata          JSONB NOT NULL,
  status            VARCHAR(50) NOT NULL,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at        TIMESTAMP
)

service_order_items (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_order_id  UUID NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
  history_id        UUID NULL,
  item_type         VARCHAR(20) NOT NULL,
  reference_id      UUID NOT NULL,
  name              VARCHAR(200) NOT NULL,
  quantity          INTEGER NOT NULL,
  unit_price        BIGINT NOT NULL,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at        TIMESTAMP
)
```

**`migrations/002_payment_fields.sql`** — campos de pagamento:

```sql
-- Adicionados à tabela service_orders:
mp_preference_id  VARCHAR(255) NULL,
mp_payment_id     VARCHAR(255) NULL,
payment_url       TEXT NULL
```

### DynamoDB — tabela `order_history`

Partition key: `order_id` (UUID). Sort key: `occurred_at` (ISODate).

```json
{
  "order_id": "uuid",
  "occurred_at": "2025-05-17T10:00:00Z",
  "previous_status": "DIAGNOSING",
  "new_status": "PENDING_AUTHORIZATION",
  "changed_by": "user_id",
  "notes": "string",
  "snapshot": {
    "items": [
      {
        "item_type": "PRODUCT",
        "reference_id": "uuid",
        "name": "Filtro de óleo",
        "quantity": 2,
        "unit_price": 4500
      }
    ],
    "total_amount": 24000,
    "customer_id": "uuid",
    "vehicle_id": "uuid"
  }
}
```

---

## Variáveis de Ambiente

Copiar `.env.example` para `.env` e preencher antes de rodar localmente.

```env
# Servidor
PORT=8082

# Banco de Dados PostgreSQL
DB_HOST=localhost
DB_PORT=5434
DB_USER=oficina
DB_PASSWORD=oficina
DB_NAME=db_ms2
DB_SSLMODE=disable

# Autenticação JWT (mesmo segredo compartilhado com MS1 e MS3)
JWT_SECRET_KEY=local-dev-secret

# Endereços dos outros microsserviços
MS1_BASE_URL=http://localhost:8081
MS3_BASE_URL=http://localhost:8083

# AWS (LocalStack em desenvolvimento)
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_ENDPOINT_URL=http://localhost:4566

# Filas SQS
ORDER_INVENTORY_OP_REQUESTED_QUEUE_URL=http://localhost:4566/000000000000/order-inventory-op-requested
ORDER_INVENTORY_OP_SUCCEEDED_QUEUE_URL=http://localhost:4566/000000000000/order-inventory-op-succeeded
ORDER_INVENTORY_OP_FAILED_QUEUE_URL=http://localhost:4566/000000000000/order-inventory-op-failed
CUSTOMER_DELETED_QUEUE_URL=http://localhost:4566/000000000000/customer-deleted

# Mercado Pago
MP_ACCESS_TOKEN=APP_USR-...
MP_WEBHOOK_SECRET=seu-webhook-secret
MP_NOTIFICATION_URL=https://api.oficina-tech.com/payments/mp-webhook

# SMTP (notificações por email)
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=noreply@oficina-tech.com
SMTP_PASSWORD=senha-smtp
SMTP_FROM=noreply@oficina-tech.com

# Observabilidade (opcional)
OTEL_EXPORTER_OTLP_ENDPOINT=
```

Em produção, `AWS_SESSION_TOKEN` é necessário para credenciais AWS Academy. Remover `AWS_ENDPOINT_URL` para apontar para a AWS real.

---

## BDD — Godog

Testes de comportamento em `features/` (Gherkin). Runner e step definitions em `bdd/`.

| Feature | Cenários cobertos |
|---------|------------------|
| `service_order_lifecycle.feature` | Ciclo completo `RECEIVED → DELIVERED` |
| `saga_compensation.feature` | Estoque insuficiente, autorização negada, cancelamento |
| `payment_flow.feature` | `COMPLETED → AWAITING_PAYMENT → PAID` via webhook MP |
| `customer_deleted.feature` | Cancelamento automático de OS ao deletar cliente |
| `saga_recovery.feature` | Recuperação de saga interrompida após reinício do serviço |

```bash
# Sobe ambiente E2E (todos os serviços + LocalStack + mock Mercado Pago)
docker-compose -f docker-compose.e2e.yml up -d

# Roda os testes BDD
cd bdd && go test -v ./...
```

Variáveis necessárias para BDD: `MS1_URL`, `MS2_URL`, `MS3_URL`, `MP_MOCK_URL`, `MP_WEBHOOK_SECRET`.

---

## Como Rodar Localmente

```bash
# 1. Sobe PostgreSQL (porta 5434) + LocalStack (SQS + DynamoDB)
docker-compose up -d

# 2. Instala dependências Go
go mod download

# 3. Copia e preenche o .env
cp .env.example .env

# 4. Roda o serviço
go run cmd/api/main.go
```

A API fica disponível em `http://localhost:8082`.

Swagger disponível em `http://localhost:8082/swagger/index.html`.

---

## Testes

```bash
# Testes unitários (máquina de estados, saga orchestrator, usecases)
go test ./internal/... -v -coverprofile=coverage.out -covermode=atomic

# Cobertura mínima exigida: 80%
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# BDD (ambiente E2E necessário)
cd bdd && go test -v ./...

# Lint
go vet ./...
```

---

## CI/CD

O pipeline está em `.github/workflows/` e segue quatro workflows independentes:

### `ci.yml` — Integração Contínua

Disparado em Pull Requests para `develop` e `main`, e em push para `develop`.

Etapas:
1. `go vet ./...` — análise estática
2. `golangci-lint` — linting
3. `go test ./internal/... -coverprofile=coverage.out` — testes unitários
4. Verificação de cobertura mínima (threshold configurado em 5% no CI; 80% por convenção do projeto)

### `deploy.yml` — Deploy em Produção

Disparado quando um PR de branch `release/*` é mergeado na `main`, ou via `workflow_dispatch`.

Estágios:

| Stage | Job | Descrição |
|-------|-----|-----------|
| 1 | `build-test` | Compila e testa; calcula versão via Conventional Commits |
| 2 | `build-docker` | Constrói imagem Docker e publica no GHCR (`ghcr.io/{repo}:{version}`) |
| 3 | `deploy-k8s` | Lê TF state do S3 para obter cluster EKS, RDS e NLB; aplica manifestos K8s; aguarda health check em `/health`; cria tag Git somente após deploy confirmado |
| 4 | `github-release` | Publica GitHub Release com changelog gerado |

### `release.yml` — Criação de Release PR

Calcula a próxima versão semântica com base nos commits (`feat:` → minor, `feat!:` → major, demais → patch) e abre um PR `release/vX.Y.Z` para `main`.

### `rollback.yml` — Rollback Manual

Disparado via `workflow_dispatch` com input `version` (formato `vX.Y.Z`).

- Valida que a tag e a imagem Docker existem no GHCR
- Faz checkout na tag especificada (usa manifestos K8s daquela versão exata)
- Lê TF state atual do S3 (infraestrutura live)
- Redeploya a imagem da tag no cluster sem criar nova tag

---

## Deploy Kubernetes

Manifestos em `k8s/`:

| Arquivo | Descrição |
|---------|-----------|
| `namespace.yaml` | Namespace `app-oficina-tech` |
| `deployment.yaml` | Deployment com 1 réplica inicial; estratégia `RollingUpdate` (`maxSurge: 1`, `maxUnavailable: 0`); recursos: `requests: 256Mi/250m`, `limits: 768Mi/500m`; probes em `/health` |
| `service.yaml` | `ClusterIP` expondo porta `8082` |
| `configmap.yaml` | Variáveis de ambiente não-secretas |
| `secret.yaml.example` | Template para JWT, DB, MP e SMTP credentials |
| `hpa.yaml` | HPA `minReplicas: 1`, `maxReplicas: 5`, target CPU `70%` |
| `datadog/` | Configuração do agente Datadog para observabilidade |

O `deployment.yaml` injeta `DD_AGENT_HOST` via `fieldRef: status.hostIP` e configura `OTEL_EXPORTER_OTLP_ENDPOINT` automaticamente para o agente Datadog na porta `4317`.

Probes:
- **Startup probe**: `/health`, `periodSeconds: 5`, `failureThreshold: 36` (até 3 minutos)
- **Liveness probe**: `/health`, `initialDelaySeconds: 60`, `periodSeconds: 10`
- **Readiness probe**: `/health`, `initialDelaySeconds: 20`, `periodSeconds: 5`
