-- Migration 003: Preferences API → Orders API (SDK github.com/mercadopago/sdk-go)
-- Adiciona snapshot de customer para o Payer do MP e novo status PAYMENT_REJECTED.
--
-- Rollback: ver 003_mp_orders_api_rollback.sql

BEGIN;

-- Renomeia campo principal (Preferences → Orders)
ALTER TABLE service_orders RENAME COLUMN mp_preference_id TO mp_order_id;

-- Atualiza índice para refletir novo nome
DROP INDEX IF EXISTS idx_service_orders_mp_preference_id;
CREATE INDEX IF NOT EXISTS idx_service_orders_mp_order_id ON service_orders(mp_order_id);

-- Cache do status do Order no MP (evita GetOrder repetido no webhook)
ALTER TABLE service_orders ADD COLUMN mp_order_status VARCHAR(50) NULL;

-- Status detail retornado pelo MP quando pagamento é rejeitado
ALTER TABLE service_orders ADD COLUMN payment_rejection_reason VARCHAR(255) NULL;

-- Snapshot do customer: populado na criação da OS via REST sync único com MS1.
-- Elimina chamada extra ao MS1 no momento do pagamento.
ALTER TABLE service_orders ADD COLUMN customer_email VARCHAR(255) NULL;
ALTER TABLE service_orders ADD COLUMN customer_cpf   VARCHAR(20)  NULL;
ALTER TABLE service_orders ADD COLUMN customer_name  VARCHAR(255) NULL;

COMMIT;
