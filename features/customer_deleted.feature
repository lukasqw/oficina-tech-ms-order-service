# language: pt
Funcionalidade: Customer deletado dispara cancelamento de OS

  Cenário: Customer com OS em IN_PROGRESS é deletado no MS1
    Dado uma OS em IN_PROGRESS de customer X
    Quando customer X é deletado no MS1
    Então MS2 cancela a OS via CANCEL_RESERVED
    E MS3 libera estoque

  Cenário: Customer com OS em PENDING_AUTHORIZATION é deletado — CANCEL_RESERVED disparado
    Dado uma OS em PENDING_AUTHORIZATION de customer X
    Quando customer X é deletado no MS1
    Então MS2 cancela a OS via CANCEL_RESERVED
    E MS3 libera estoque

  Cenário: Customer com múltiplas OS ativas é deletado — todas são canceladas
    Dado um cliente cadastrado com veículo registrado
    E o MS3 possui estoque suficiente para todos os produtos
    E o cliente possui duas OS ativas em IN_PROGRESS
    Quando customer X é deletado no MS1
    Então ambas as OS do cliente são canceladas

  Cenário: Customer sem OS ativas é deletado sem efeitos colaterais
    Dado um cliente cadastrado com veículo registrado
    Quando customer X é deletado no MS1
    Então o evento é processado sem erros e sem cancelamentos
