# Oficina Tech MS2 OS Service

## Mercado Pago

Payment integration lives in `internal/modules/billing`.

Required environment variables:

- `MP_ACCESS_TOKEN`
- `MP_WEBHOOK_SECRET`
- `MP_NOTIFICATION_URL`

When a service order advances from `COMPLETED`, MS2 creates a Mercado Pago checkout preference, stores `mp_preference_id` and `payment_url`, and moves the order to `AWAITING_PAYMENT` with `saga_status=AWAITING_PAYMENT`.

On startup, orders in `AWAITING_PAYMENT` are only logged. MS2 does not republish SQS messages for them because the Mercado Pago webhook will continue the payment flow.
