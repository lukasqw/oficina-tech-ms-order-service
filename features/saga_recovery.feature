# language: pt
Funcionalidade: Recuperação de falha do MS2 durante saga

  @integration
  Cenário: MS2 reinicia durante saga RESERVE em andamento
    Dado OS em DIAGNOSING, saga RESERVE iniciada
    Quando MS2 reinicia antes de receber resultado
    Então ao subir, MS2 republica evento
    E MS3 (idempotente) republica resultado já processado
    E a OS avança para PENDING_AUTHORIZATION

  @integration
  Cenário: MS2 reinicia durante saga RESERVED_DECREASE em andamento
    Dado OS em IN_PROGRESS, saga RESERVED_DECREASE iniciada
    Quando MS2 reinicia antes de receber resultado do RESERVED_DECREASE
    Então ao subir, MS2 republica evento de RESERVED_DECREASE
    E MS3 (idempotente) republica resultado já processado
    E a OS avança para COMPLETED

  @integration
  Cenário: MS2 reinicia com saga_status AWAITING_PAYMENT — não republica SQS
    Dado uma OS em AWAITING_PAYMENT via preferência MP criada
    Quando MS2 reinicia antes de receber o webhook do MP
    Então a OS permanece em AWAITING_PAYMENT
    E o webhook do MP retoma o fluxo normalmente
    E a OS avança para PAID

  @integration
  Cenário: Evento de saga duplicado é descartado por idempotência
    Dado OS em DIAGNOSING, saga RESERVE iniciada
    Quando MS2 recebe o mesmo evento de sucesso duas vezes com o mesmo saga_id
    Então a OS avança apenas uma vez para PENDING_AUTHORIZATION
