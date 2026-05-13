CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS service_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id UUID NOT NULL,
  vehicle_id UUID NOT NULL,
  description TEXT,
  status VARCHAR(50) NOT NULL,
  saga_status VARCHAR(50) NOT NULL DEFAULT 'IDLE',
  current_saga_id UUID NULL,
  saga_target_status VARCHAR(50) NULL,
  saga_notes TEXT NULL,
  closed_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_service_orders_customer_id ON service_orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_service_orders_vehicle_id ON service_orders(vehicle_id);
CREATE INDEX IF NOT EXISTS idx_service_orders_status ON service_orders(status);
CREATE INDEX IF NOT EXISTS idx_service_orders_saga_status ON service_orders(saga_status);
CREATE INDEX IF NOT EXISTS idx_service_orders_current_saga_id ON service_orders(current_saga_id);
CREATE INDEX IF NOT EXISTS idx_service_orders_deleted_at ON service_orders(deleted_at);

CREATE TABLE IF NOT EXISTS service_order_histories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_order_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  status VARCHAR(50) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_service_order_histories_service_order_id ON service_order_histories(service_order_id);

CREATE TABLE IF NOT EXISTS service_order_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_order_id UUID NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
  history_id UUID NULL,
  item_type VARCHAR(20) NOT NULL,
  reference_id UUID NOT NULL,
  name VARCHAR(200) NOT NULL,
  quantity INTEGER NOT NULL,
  unit_price BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_service_order_items_service_order_id ON service_order_items(service_order_id);
CREATE INDEX IF NOT EXISTS idx_service_order_items_history_id ON service_order_items(history_id);
CREATE INDEX IF NOT EXISTS idx_service_order_items_deleted_at ON service_order_items(deleted_at);
