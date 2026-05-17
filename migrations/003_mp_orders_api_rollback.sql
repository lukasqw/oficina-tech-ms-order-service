-- Rollback da migration 003
-- Restaura campo mp_preference_id; mantém colunas customer_* (sem impacto na versão anterior)

BEGIN;

ALTER TABLE service_orders RENAME COLUMN mp_order_id TO mp_preference_id;

DROP INDEX IF EXISTS idx_service_orders_mp_order_id;
CREATE INDEX IF NOT EXISTS idx_service_orders_mp_preference_id ON service_orders(mp_preference_id);

ALTER TABLE service_orders DROP COLUMN IF EXISTS mp_order_status;
ALTER TABLE service_orders DROP COLUMN IF EXISTS payment_rejection_reason;

COMMIT;
