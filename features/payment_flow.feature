# language: pt
Funcionalidade: Pagamento via Mercado Pago (Orders API)

  @integration
  Cenário: Pagamento aprovado via webhook MP
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    E a OS possui URL de pagamento
    Quando MP webhook chega com status=approved (mock dispara)
    Então a OS está em PAID

  @integration
  Cenário: Pagamento rejeitado via webhook MP — OS vai para PAYMENT_REJECTED
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=rejected (mock dispara)
    Então a OS está em PAYMENT_REJECTED

  @integration
  Cenário: Webhook MP com assinatura inválida é rejeitado
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com assinatura inválida
    Então o webhook é rejeitado com status 4xx
    E a OS permanece em AWAITING_PAYMENT

  @integration
  Cenário: Retry de pagamento após rejeição volta OS para AWAITING_PAYMENT
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=rejected (mock dispara)
    Então a OS está em PAYMENT_REJECTED
    Quando retry-payment é chamado
    Então a OS está em AWAITING_PAYMENT

  @integration
  Cenário: Cancelamento de OS em AWAITING_PAYMENT cancela Order no MP
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    Quando cliente cancela
    Então o mock recebeu chamada de cancel no MP
    E a OS está em CANCELED

  @integration
  Cenário: Cancelamento de OS em PAID faz refund no MP
    Dado uma OS em COMPLETED com estoque reservado
    Quando advance cria Order MP (mock retorna mp_order_id)
    Então a OS está em AWAITING_PAYMENT
    Quando MP webhook chega com status=approved (mock dispara)
    Então a OS está em PAID
    Quando cliente cancela
    Então o mock recebeu chamada de refund no MP
    E a OS está em CANCELED
