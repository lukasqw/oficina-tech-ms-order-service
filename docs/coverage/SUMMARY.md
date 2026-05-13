# Sessão F — Resumo de Cobertura por Serviço

> Gerado pela execução das suítes unitárias dos três serviços + estrutura BDD/E2E (Godog) plugada na MS2.
> A suíte BDD vive em [`bdd/`](../../bdd) e exercita os 3 serviços em conjunto via `docker-compose.e2e.yml`.

| Serviço | Foco | Cobertura | Meta | Resultado |
|---|---|---|---|---|
| **MS1** módulos `access_control` + `customers` + `vehicles` | autenticação, CRUD customers/vehicles | **81.7 %** | ≥ 80 % | ✅ |
| **MS3** módulo `inventory` (produto + estoque + saga_operation) | reserve / decrease / cancel + idempotência | **81.4 %** | ≥ 80 % | ✅ |
| **MS2** módulo `billing` (Mercado Pago) | preference + webhook + assinatura | **85.8 %** | ≥ 80 % | ✅ |
| **MS2** módulo `service_order/application/saga` | orquestrador + transições | **80.5 %** | ≥ 80 % | ✅ |
| **MS2** módulo `service_order` agregado (use cases + handlers + persistence) | máquina de estados completa | **19.4 %** | ≥ 80 % | ❌ |

## O que falta para fechar 80 % do MS2 `service_order`

A Sessão D entregou o *saga orchestrator* e o domínio `saga_state` com testes (`80.5 %`), mas
**os use cases e a camada HTTP/persistência do `service_order` ficaram sem testes unitários**.
Pacotes 0 % atualmente:

- `internal/modules/service_order/application/usecases/` (9 use cases: create, update, delete, advance, get, get_all, get_history, respond_to_authorization)
- `internal/modules/service_order/infra/http/handlers/`
- `internal/modules/service_order/infra/http/dto/`
- `internal/modules/service_order/infra/persistence/`
- `internal/modules/service_order/infra/adapters/implementations/`

Esses testes são pré-requisito de Sessão D que não veio nesta wave; estão fora do escopo da Sessão F (BDD + E2E).
A suíte BDD escrita aqui exercita esses caminhos *em runtime* contra a stack real, mas o
relatório `go test -cover` não conta cobertura observada via E2E.

## Arquivos gerados

- `coverage.out` / `coverage.html` — perfil agregado (incluindo `internal/shared`).
- `modules.out` — apenas pacotes em `internal/modules/`.
- `billing.out` — perfil isolado do módulo Mercado Pago.
- `saga.out` — perfil isolado do orquestrador + domínio.

Comando para regenerar:

```sh
go test -count=1 -covermode=atomic -coverprofile=docs/coverage/coverage.out ./internal/...
go tool cover -html=docs/coverage/coverage.out -o docs/coverage/coverage.html
```

## Estrutura BDD (Godog)

```
features/
├── service_order_lifecycle.feature   # F.2 + F.6 (happy path + cancel pós-COMPLETED)
├── saga_compensation.feature         # F.3 + F.4 + F.5 (estoque insuficiente / negação / cancel em IN_PROGRESS)
├── customer_deleted.feature          # F.7
├── payment_flow.feature              # F.8 (MP webhook mock)
└── saga_recovery.feature             # F.9 (restart MS2 mid-saga)

bdd/
├── main_test.go                      # entry point Godog
├── steps/
│   ├── world.go                      # estado por cenário (URLs, JWT, IDs)
│   ├── http_helpers.go               # envelope decode, login admin, HMAC do MP
│   ├── service_order_steps.go        # background + ações do ciclo de vida
│   ├── saga_steps.go                 # asserções de saga e estoque
│   ├── payment_steps.go              # preference + webhook MP
│   └── recovery_steps.go             # restart MS2 mid-saga
└── mockmp/                           # servidor HTTP que simula Mercado Pago
```

Como executar:

```sh
docker compose -f docker-compose.e2e.yml up -d --build
# Espere os 3 serviços ficarem healthy (cerca de 30s)
go test ./bdd/... -count=1 -v
```
