# language: pt
Funcionalidade: Pagamento via Mercado Pago (mock)

  @integration
  Cenário: Pagamento aprovado via webhook MP
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=approved (mock dispara)
    Então a OS está em PAID

  @integration
  Cenário: Pagamento rejeitado via webhook MP — OS permanece em AWAITING_PAYMENT
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=rejected (mock dispara)
    Então a OS permanece em AWAITING_PAYMENT

  @integration
  Cenário: Webhook MP com assinatura inválida é rejeitado
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance dispara criação de preferência MP (MP mock retorna preference_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com assinatura inválida
    Então o webhook é rejeitado com status 4xx
    E a OS permanece em AWAITING_PAYMENT
