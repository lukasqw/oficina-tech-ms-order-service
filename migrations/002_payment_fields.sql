ALTER TABLE service_orders
  ADD COLUMN IF NOT EXISTS mp_preference_id VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS mp_payment_id VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS payment_url TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_service_orders_mp_preference_id ON service_orders(mp_preference_id);
CREATE INDEX IF NOT EXISTS idx_service_orders_mp_payment_id ON service_orders(mp_payment_id);
