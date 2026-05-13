# JWT Spec

Especificacao compartilhada de JWT para os microsservicos Oficina Tech.

Referencias do monolito:
- `internal/modules/access_control/infra/auth/jwt_service.go`
- `internal/modules/access_control/domain/auth/jwt_service.go`
- `internal/shared/infra/http/middleware/auth_middleware.go`

## Decisao

O MS1 emite tokens JWT. MS1, MS2 e MS3 validam tokens localmente usando a mesma chave simetrica.

Nenhum servico deve chamar o MS1 em runtime para validar tokens.

## Chave

- Nome: `JWT_SECRET_KEY`
- Producao: compartilhada via AWS Secrets Manager ou variavel de ambiente injetada pelo orquestrador/K8s.
- Desenvolvimento local: variavel de ambiente.
- A chave deve ser omitida dos repositorios e pipelines.

## Algoritmo

- `alg`: `HS256`
- Bibliotecas Go devem manter compatibilidade com `github.com/golang-jwt/jwt/v5`.
- Validadores devem rejeitar tokens com algoritmo diferente de HMAC.

## Header HTTP

Endpoints protegidos recebem:

```http
Authorization: Bearer <jwt>
```

Erros:

- Header ausente: `401` com `UNAUTHORIZED`
- Formato invalido: `401` com `UNAUTHORIZED`
- Token invalido ou expirado: `401` com `UNAUTHORIZED`

## Claims Obrigatorios

O plano mestre exige os claims obrigatorios abaixo:

| Claim | Tipo | Origem | Observacao |
|---|---|---|---|
| `sub` | string UUID | user_id | Identificador canonico do usuario |
| `role` | string | role | Papel do usuario |
| `exp` | NumericDate | expiracao | Obrigatorio para expiracao local |

Para compatibilidade com o monolito durante a extracao, o emissor tambem deve preencher:

| Claim | Tipo | Origem |
|---|---|---|
| `user_id` | string UUID | `TokenClaims.UserID` |
| `name` | string | `TokenClaims.Name` |
| `email` | string email | `TokenClaims.Email` |
| `iat` | NumericDate | emitido em |

Validadores novos devem preferir `sub` como identificador canonico. Durante a migracao, se `sub` estiver ausente e `user_id` estiver presente, o servico pode aceitar `user_id` apenas para compatibilidade com tokens emitidos pelo codigo legado.

## Roles

Roles validas no monolito:

```text
ADMIN
MANAGER
USER
```

O documento de arquitetura nomeia responsabilidades alvo como `ADMIN`, `MECHANIC` e `CUSTOMER`. A normalizacao final de roles deve acontecer no MS1 durante a extracao. Ate essa decisao ser implementada, os servicos devem tratar `role` como string validada contra as roles emitidas pelo MS1.

## Duracao

- Tokens emitidos pelo monolito expiram em 24 horas.
- MS1 deve manter duracao de 24 horas salvo decisao posterior em ADR propria.

## Payload Exemplo

```json
{
  "sub": "a98c93a0-67fa-489a-bd7d-6bb7c3ef0d35",
  "user_id": "a98c93a0-67fa-489a-bd7d-6bb7c3ef0d35",
  "name": "Admin User",
  "email": "admin@example.com",
  "role": "ADMIN",
  "iat": 1777576800,
  "exp": 1777663200
}
```

## Contexto HTTP

Apos validar o token, o middleware deve inserir no contexto da request:

| Chave | Valor |
|---|---|
| `user_id` | `sub` ou `user_id` legado |
| `user_email` | `email` |
| `user_role` | `role` |

## Regras de Validacao Local

1. Extrair `Authorization`.
2. Validar formato `Bearer <token>`.
3. Validar assinatura HS256 com `JWT_SECRET_KEY`.
4. Validar `exp`.
5. Extrair `sub`, `role` e claims auxiliares.
6. Rejeitar token sem `role`.
7. Rejeitar token sem identificador de usuario (`sub` ou `user_id` legado).
