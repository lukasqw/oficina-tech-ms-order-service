# ADR 0001: Mensageria com SQS e SNS

## Status

Aceito.

## Contexto

A refatoracao do monolito Oficina Tech divide o sistema em tres microsservicos:

- MS1 Identity & Client
- MS2 OS Service
- MS3 Workshop

O MS2 orquestra o ciclo de vida da ordem de servico e precisa coordenar operacoes de estoque com o MS3. O MS1 publica exclusoes de clientes para que o MS2 possa reagir sem acoplamento sincrono. O MS3 tambem precisa publicar alertas de estoque baixo para consumo futuro por multiplos assinantes.

## Decisao

Usar Amazon SQS standard para mensagens ponto a ponto ligadas a comandos/eventos da saga e Amazon SNS para fan-out de alertas.

Filas SQS:

- `customer-deleted`
- `order-inventory-op-requested`
- `order-inventory-op-succeeded`
- `order-inventory-op-failed`

Topico SNS:

- `inventory-low-alert`

Cada fila SQS tera DLQ propria, `visibility_timeout_seconds = 30`, retencao de 14 dias e `maxReceiveCount = 3`.

## Justificativa

SQS standard atende a comunicacao assíncrona da saga com baixo acoplamento, retry nativo e DLQ por fila. A saga nao depende de ordenacao global estrita; a correlacao por `saga_id` e `order_id` e suficiente para idempotencia e reconciliacao.

SNS e usado para `inventory-low-alert` porque alertas de estoque baixo sao eventos de fan-out: hoje nao ha consumidor obrigatorio, mas o contrato permite adicionar notificacoes, dashboards ou automacoes sem alterar o MS3.

## Desenvolvimento Local

Cada repositorio deve executar LocalStack no `docker-compose.yml` com SQS, SNS e DynamoDB habilitados no mesmo container. Em dev, os servicos usam:

```env
AWS_ENDPOINT_URL=http://localstack:4566
AWS_REGION=us-east-1
```

Em producao, `AWS_ENDPOINT_URL` deve ser omitida para que o SDK use os endpoints reais da AWS.

## SDK

Os servicos Go devem usar AWS SDK Go v2:

```text
github.com/aws/aws-sdk-go-v2/service/sqs
github.com/aws/aws-sdk-go-v2/service/sns
github.com/aws/aws-sdk-go-v2/service/dynamodb
```

## Permissoes Minimas

- MS1 pode publicar em `customer-deleted`.
- MS2 pode consumir `customer-deleted`, publicar em `order-inventory-op-requested`, consumir `order-inventory-op-succeeded` e consumir `order-inventory-op-failed`.
- MS3 pode consumir `order-inventory-op-requested`, publicar em `order-inventory-op-succeeded`, publicar em `order-inventory-op-failed` e publicar no topico `inventory-low-alert`.

## Consequencias

- Cada consumer deve validar o contrato JSON antes de executar regra de negocio.
- Falhas transientes devem retornar erro ao consumer para permitir retry pelo SQS.
- Mensagens invalidas ou que excedam tres tentativas devem seguir para DLQ.
- Consumers devem ser idempotentes por `saga_id`, `order_id` e `operation`.
