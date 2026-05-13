# language: pt
Funcionalidade: Recuperação de falha do MS2 durante saga

  Cenário: MS2 reinicia durante saga RESERVE em andamento
    Dado OS em DIAGNOSING, saga RESERVE iniciada
    Quando MS2 reinicia antes de receber resultado
    Então ao subir, MS2 republica evento
    E MS3 (idempotente) republica resultado já processado
    E a OS avança para PENDING_AUTHORIZATION
