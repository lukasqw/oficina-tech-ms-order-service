# language: pt
Funcionalidade: Ciclo completo de Ordem de Serviço

  @integration
  Cenário: OS aprovada e executada com sucesso
    Dado um cliente cadastrado com veículo registrado
    E o MS3 possui estoque suficiente para todos os produtos
    Quando o cliente abre uma OS com troca de óleo e filtro de ar
    Então a OS é criada com status RECEIVED
    Quando o mecânico avança a OS para DIAGNOSING
    Então a OS está em DIAGNOSING
    Quando o mecânico avança a OS para PENDING_AUTHORIZATION
    Então o MS3 recebe comando de RESERVE para os produtos
    E o MS3 confirma a reserva com sucesso
    E a OS avança para PENDING_AUTHORIZATION
    Quando o cliente aprova a OS
    Então a OS avança para AUTHORIZED
    Quando o mecânico avança a OS para IN_PROGRESS
    Então a OS está em IN_PROGRESS
    Quando o mecânico avança a OS para COMPLETED
    Então o MS3 recebe comando de RESERVED_DECREASE para os produtos
    E o MS3 confirma a baixa com sucesso
    E a OS avança para COMPLETED
    E o cliente recebe notificação por email

  @integration
  Cenário: Cliente visualiza suas ordens e aprova a que está em pendência de autorização
    Dado um cliente cadastrado com veículo registrado
    E o MS3 possui estoque suficiente para todos os produtos
    Quando o cliente abre uma OS com troca de óleo e filtro de ar
    E o mecânico avança a OS para DIAGNOSING
    E o mecânico avança a OS para PENDING_AUTHORIZATION
    E o MS3 confirma a reserva com sucesso
    E a OS avança para PENDING_AUTHORIZATION
    Quando o cliente faz login no sistema
    E o cliente lista suas ordens de serviço
    Então a OS aparece na lista com status PENDING_AUTHORIZATION
    Quando o cliente aprova a OS
    Então a OS avança para AUTHORIZED

  @integration
  Cenário: Cancelamento pós-COMPLETED dispara CANCEL_CONFIRMED
    Dado uma OS em COMPLETED com estoque reservado
    Quando o cliente cancela a OS
    Então o MS3 recebe CANCEL_CONFIRMED
    E a OS avança para CANCELED
    E estoque retorna para available

  @integration
  Cenário: Ciclo completo até entrega — PAID a DELIVERED
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=approved (mock dispara)
    Então a OS está em PAID
    Quando o mecânico avança a OS para DELIVERED
    Então a OS está em DELIVERED

  @integration
  Cenário: Mecânico atualiza itens da OS em RECEIVED
    Dado um cliente cadastrado com veículo registrado
    E o MS3 possui estoque suficiente para todos os produtos
    Quando o cliente abre uma OS com troca de óleo e filtro de ar
    Então a OS é criada com status RECEIVED
    Quando o mecânico atualiza os itens da OS
    Então a atualização de itens é aceita

  @integration
  Cenário: Atualização de itens bloqueada após PENDING_AUTHORIZATION
    Dado uma OS em PENDING_AUTHORIZATION com estoque reservado
    Quando o mecânico tenta atualizar os itens da OS
    Então a atualização de itens é rejeitada com erro de imutabilidade

  @integration
  Cenário: Histórico de status consultado via DynamoDB
    Dado um cliente cadastrado com veículo registrado
    E o MS3 possui estoque suficiente para todos os produtos
    Quando o cliente abre uma OS com troca de óleo e filtro de ar
    E o mecânico avança a OS para DIAGNOSING
    E o mecânico avança a OS para PENDING_AUTHORIZATION
    E o MS3 confirma a reserva com sucesso
    E a OS avança para PENDING_AUTHORIZATION
    Então o histórico da OS possui 2 ou mais entradas

  @integration
  Cenário: URL de pagamento disponível após AWAITING_PAYMENT
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS possui URL de pagamento
