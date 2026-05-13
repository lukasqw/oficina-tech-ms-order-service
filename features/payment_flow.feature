# language: pt
Funcionalidade: Pagamento via Mercado Pago (mock)

  Cenário: Pagamento aprovado via webhook MP
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=approved (mock dispara)
    Então a OS está em PAID
