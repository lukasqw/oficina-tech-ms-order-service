# Event Contracts

Contratos de eventos compartilhados entre os microsservicos Oficina Tech.

Referencias:
- Plano mestre Wave 0 / T0.3
- Documento de arquitetura `docs/microservices-architecture-prompt.md`

## Convencoes

- Todos os timestamps usam RFC3339 em UTC.
- IDs usam UUID em formato string.
- Eventos publicados em SQS usam filas standard.
- O campo `event` deve ser constante por contrato.
- Consumers devem rejeitar mensagens que nao validem o JSON Schema e enviar para DLQ apos o limite configurado de tentativas.

## Recursos

| Recurso AWS | Nome | Publisher | Consumer |
|---|---|---|---|
| SQS Queue | `customer-deleted` | MS1 | MS2 |
| SQS Queue | `order-inventory-op-requested` | MS2 | MS3 |
| SQS Queue | `order-inventory-op-succeeded` | MS3 | MS2 |
| SQS Queue | `order-inventory-op-failed` | MS3 | MS2 |
| SNS Topic | `inventory-low-alert` | MS3 | fan-out futuro |

## SQS: customer-deleted

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://oficina-tech/contracts/events/customer-deleted.schema.json",
  "title": "CustomerDeleted",
  "type": "object",
  "additionalProperties": false,
  "required": ["event", "customer_id", "email", "occurred_at"],
  "properties": {
    "event": { "const": "CustomerDeleted" },
    "customer_id": { "type": "string", "format": "uuid" },
    "email": { "type": "string", "format": "email" },
    "occurred_at": { "type": "string", "format": "date-time" }
  }
}
```

### Example

```json
{
  "event": "CustomerDeleted",
  "customer_id": "2e6e6dc5-6b72-4f85-9d98-4d85351e178f",
  "email": "maria.silva@example.com",
  "occurred_at": "2026-04-30T19:20:00Z"
}
```

## SQS: order-inventory-op-requested

Operacoes validas em `operation`:

```text
RESERVE
RESERVED_DECREASE
CANCEL_RESERVED
CANCEL_CONFIRMED
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://oficina-tech/contracts/events/order-inventory-op-requested.schema.json",
  "title": "OrderInventoryOperationRequested",
  "type": "object",
  "additionalProperties": false,
  "required": ["event", "saga_id", "order_id", "operation", "items", "occurred_at"],
  "properties": {
    "event": { "const": "OrderInventoryOperationRequested" },
    "saga_id": { "type": "string", "format": "uuid" },
    "order_id": { "type": "string", "format": "uuid" },
    "operation": {
      "type": "string",
      "enum": ["RESERVE", "RESERVED_DECREASE", "CANCEL_RESERVED", "CANCEL_CONFIRMED"]
    },
    "items": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["product_id", "quantity"],
        "properties": {
          "product_id": { "type": "string", "format": "uuid" },
          "quantity": { "type": "integer", "minimum": 1 }
        }
      }
    },
    "occurred_at": { "type": "string", "format": "date-time" }
  }
}
```

### Example

```json
{
  "event": "OrderInventoryOperationRequested",
  "saga_id": "8f1d2a8d-565d-4c4e-8a83-40a3b60e60b3",
  "order_id": "7702ef2d-4d77-4643-b94f-9855ed58a734",
  "operation": "RESERVE",
  "items": [
    {
      "product_id": "c89c9e5d-9d6b-4e78-95ab-19bff9b5f3d5",
      "quantity": 2
    }
  ],
  "occurred_at": "2026-04-30T19:21:00Z"
}
```

## SQS: order-inventory-op-succeeded

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://oficina-tech/contracts/events/order-inventory-op-succeeded.schema.json",
  "title": "OrderInventoryOperationSucceeded",
  "type": "object",
  "additionalProperties": false,
  "required": ["event", "saga_id", "order_id", "operation", "occurred_at"],
  "properties": {
    "event": { "const": "OrderInventoryOperationSucceeded" },
    "saga_id": { "type": "string", "format": "uuid" },
    "order_id": { "type": "string", "format": "uuid" },
    "operation": {
      "type": "string",
      "enum": ["RESERVE", "RESERVED_DECREASE", "CANCEL_RESERVED", "CANCEL_CONFIRMED"]
    },
    "occurred_at": { "type": "string", "format": "date-time" }
  }
}
```

### Example

```json
{
  "event": "OrderInventoryOperationSucceeded",
  "saga_id": "8f1d2a8d-565d-4c4e-8a83-40a3b60e60b3",
  "order_id": "7702ef2d-4d77-4643-b94f-9855ed58a734",
  "operation": "RESERVE",
  "occurred_at": "2026-04-30T19:21:04Z"
}
```

## SQS: order-inventory-op-failed

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://oficina-tech/contracts/events/order-inventory-op-failed.schema.json",
  "title": "OrderInventoryOperationFailed",
  "type": "object",
  "additionalProperties": false,
  "required": ["event", "saga_id", "order_id", "operation", "reason", "occurred_at"],
  "properties": {
    "event": { "const": "OrderInventoryOperationFailed" },
    "saga_id": { "type": "string", "format": "uuid" },
    "order_id": { "type": "string", "format": "uuid" },
    "operation": {
      "type": "string",
      "enum": ["RESERVE", "RESERVED_DECREASE", "CANCEL_RESERVED", "CANCEL_CONFIRMED"]
    },
    "reason": { "type": "string", "minLength": 1 },
    "occurred_at": { "type": "string", "format": "date-time" }
  }
}
```

### Example

```json
{
  "event": "OrderInventoryOperationFailed",
  "saga_id": "8f1d2a8d-565d-4c4e-8a83-40a3b60e60b3",
  "order_id": "7702ef2d-4d77-4643-b94f-9855ed58a734",
  "operation": "RESERVE",
  "reason": "insufficient available stock for product c89c9e5d-9d6b-4e78-95ab-19bff9b5f3d5",
  "occurred_at": "2026-04-30T19:21:04Z"
}
```

## SNS: inventory-low-alert

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://oficina-tech/contracts/events/inventory-low-alert.schema.json",
  "title": "InventoryLowAlert",
  "type": "object",
  "additionalProperties": false,
  "required": ["event", "product_id", "product_name", "quantities", "occurred_at"],
  "properties": {
    "event": { "const": "InventoryLowAlert" },
    "product_id": { "type": "string", "format": "uuid" },
    "product_name": { "type": "string", "minLength": 1 },
    "quantities": {
      "type": "object",
      "additionalProperties": false,
      "required": ["available", "reserved", "minimum"],
      "properties": {
        "available": { "type": "integer", "minimum": 0 },
        "reserved": { "type": "integer", "minimum": 0 },
        "minimum": { "type": "integer", "minimum": 0 }
      }
    },
    "occurred_at": { "type": "string", "format": "date-time" }
  }
}
```

### Example

```json
{
  "event": "InventoryLowAlert",
  "product_id": "c89c9e5d-9d6b-4e78-95ab-19bff9b5f3d5",
  "product_name": "Filtro de oleo",
  "quantities": {
    "available": 3,
    "reserved": 2,
    "minimum": 5
  },
  "occurred_at": "2026-04-30T19:22:00Z"
}
```
