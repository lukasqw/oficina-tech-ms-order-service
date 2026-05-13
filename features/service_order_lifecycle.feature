# language: pt
Funcionalidade: Ciclo completo de Ordem de Serviço

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

  Cenário: Cancelamento pós-COMPLETED dispara CANCEL_CONFIRMED
    Dado uma OS em COMPLETED com estoque reservado
    Quando o cliente cancela a OS
    Então o MS3 recebe CANCEL_CONFIRMED
    E a OS avança para CANCELED
    E estoque retorna para available
