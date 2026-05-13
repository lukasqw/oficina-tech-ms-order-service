# language: pt
Funcionalidade: Compensações da saga de estoque

  Cenário: OS cancelada por estoque insuficiente ao tentar reservar
    Dado um cliente cadastrado com veículo registrado
    E o MS3 NÃO possui estoque suficiente para os produtos solicitados
    Quando o cliente abre uma OS com troca de óleo e filtro de ar
    E o mecânico avança a OS para DIAGNOSING
    Então a OS está em DIAGNOSING
    Quando o mecânico tenta avançar a OS de DIAGNOSING para PENDING_AUTHORIZATION
    Então o MS3 tenta reservar e falha
    E publica order-inventory-op-failed com o motivo
    E a OS permanece em DIAGNOSING
    E nenhuma reserva de estoque é mantida

  Cenário: OS com autorização negada pelo cliente
    Dado uma OS em PENDING_AUTHORIZATION com estoque reservado
    Quando o cliente nega a autorização
    Então o MS3 recebe comando de CANCEL_RESERVED
    E o MS3 libera a reserva de estoque
    E a OS avança para AUTHORIZATION_DENIED
    E o cliente recebe notificação por email

  Cenário: OS cancelada pelo cliente durante execução
    Dado uma OS em IN_PROGRESS com estoque reservado
    Quando o cliente cancela a OS
    Então o MS3 recebe comando de CANCEL_RESERVED
    E o MS3 libera a reserva de estoque
    E a OS avança para CANCELED
