# REST Contracts

Contratos compartilhados entre os microsservicos Oficina Tech.

Referencia do monolito:
- `internal/shared/dto/customer_dto.go`
- `internal/shared/dto/vehicle_dto.go`
- `internal/shared/dto/product_dto.go`
- `internal/shared/dto/service_dto.go`
- `internal/shared/utils/error_codes.go`
- `internal/shared/utils/envelope.go`
- `internal/shared/utils/http_response.go`

## Convencoes

- Todas as respostas usam o envelope padrao:

```json
{
  "data": {},
  "errors": []
}
```

- Em erro, `data` deve ser `null` e `errors` deve conter um ou mais itens:

```json
{
  "data": null,
  "errors": [
    {
      "code": "VALIDATION_FAILED",
      "message": "Mensagem em portugues",
      "field": "campo_opcional"
    }
  ]
}
```

- `field` e opcional e deve ser omitido quando o erro nao estiver associado a um campo especifico.
- IDs sao UUID em formato string.
- Valores monetarios usam inteiro em centavos.

## Autenticacao

Todos os endpoints compartilhados exigem:

```http
Authorization: Bearer <jwt>
```

Falhas de autenticacao retornam `401 Unauthorized` com `UNAUTHORIZED`.

## Codigos HTTP Padrao

| Status | Quando usar | Codigo de erro |
|---|---|---|
| 200 OK | Consulta executada com sucesso | - |
| 400 Bad Request | Payload, parametro ou UUID invalido | `INVALID_REQUEST`, `VALIDATION_FAILED`, `INVALID_UUID` |
| 401 Unauthorized | Header ausente, formato invalido, token invalido ou expirado | `UNAUTHORIZED` |
| 403 Forbidden | Usuario autenticado sem permissao | `FORBIDDEN` |
| 404 Not Found | Recurso nao encontrado | `NOT_FOUND` |
| 409 Conflict | Recurso duplicado ou conflito de negocio | `DUPLICATE_RESOURCE` |
| 500 Internal Server Error | Erro inesperado | `INTERNAL_ERROR`, `DATABASE_ERROR` |

Codigos de erro reutilizados do monolito:

```text
INVALID_REQUEST
VALIDATION_FAILED
INVALID_UUID
INVALID_CREDENTIALS
NOT_FOUND
DUPLICATE_RESOURCE
UNAUTHORIZED
FORBIDDEN
INTERNAL_ERROR
DATABASE_ERROR
```

## GET /customers/{id}

Consulta um cliente por ID.

### Response 200

Schema `CustomerDTO`, reutilizado de `internal/shared/dto/customer_dto.go`.

```json
{
  "data": {
    "id": "2e6e6dc5-6b72-4f85-9d98-4d85351e178f",
    "name": "Maria Silva",
    "email": "maria.silva@example.com",
    "phone": "+5511999999999"
  },
  "errors": []
}
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "CustomerDTO",
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "name", "email", "phone"],
  "properties": {
    "id": { "type": "string", "format": "uuid" },
    "name": { "type": "string" },
    "email": { "type": "string", "format": "email" },
    "phone": { "type": "string" }
  }
}
```

## GET /vehicles/{id}

Consulta um veiculo por ID.

### Response 200

Schema `VehicleDTO`, reutilizado de `internal/shared/dto/vehicle_dto.go`.

```json
{
  "data": {
    "id": "71ac5fb7-49c9-4e38-a92e-a3b94e70ab38",
    "customer_id": "2e6e6dc5-6b72-4f85-9d98-4d85351e178f",
    "license_plate": "ABC1D23",
    "brand": "Toyota",
    "model": "Corolla",
    "model_year": 2024,
    "manufacture_year": 2023
  },
  "errors": []
}
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "VehicleDTO",
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "customer_id", "license_plate", "brand", "model", "model_year", "manufacture_year"],
  "properties": {
    "id": { "type": "string", "format": "uuid" },
    "customer_id": { "type": "string", "format": "uuid" },
    "license_plate": { "type": "string" },
    "brand": { "type": "string" },
    "model": { "type": "string" },
    "model_year": { "type": "integer" },
    "manufacture_year": { "type": "integer" }
  }
}
```

## GET /products/{id}

Consulta um produto por ID.

### Response 200

Schema `ProductDTO`, reutilizado de `internal/shared/dto/product_dto.go`.

```json
{
  "data": {
    "id": "c89c9e5d-9d6b-4e78-95ab-19bff9b5f3d5",
    "name": "Filtro de oleo",
    "description": "Filtro de oleo para motor flex",
    "price": 4500,
    "product_type": "PART"
  },
  "errors": []
}
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "ProductDTO",
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "name", "description", "price", "product_type"],
  "properties": {
    "id": { "type": "string", "format": "uuid" },
    "name": { "type": "string" },
    "description": { "type": "string" },
    "price": { "type": "integer", "minimum": 0 },
    "product_type": { "type": "string" }
  }
}
```

## GET /services/{id}

Consulta um servico do catalogo por ID.

### Response 200

Schema `ServiceDTO`, reutilizado de `internal/shared/dto/service_dto.go`.

```json
{
  "data": {
    "id": "aaf4b55c-9c15-4dd7-90be-a8046a94fb5b",
    "name": "Troca de oleo",
    "description": "Substituicao de oleo do motor",
    "price": 15000
  },
  "errors": []
}
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "ServiceDTO",
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "name", "description", "price"],
  "properties": {
    "id": { "type": "string", "format": "uuid" },
    "name": { "type": "string" },
    "description": { "type": "string" },
    "price": { "type": "integer", "minimum": 0 }
  }
}
```
