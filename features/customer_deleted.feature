# language: pt
Funcionalidade: Customer deletado dispara cancelamento de OS

  Cenário: Customer com OS em IN_PROGRESS é deletado no MS1
    Dado uma OS em IN_PROGRESS de customer X
    Quando customer X é deletado no MS1
    Então MS2 cancela a OS via CANCEL_RESERVED
    E MS3 libera estoque
