package service_order

import "testing"

func TestSagaStateLifecycleAndIdempotency(t *testing.T) {
	order := NewTestServiceOrderWithStatus("customer", "vehicle", StatusDiagnosing)
	sagaID := "11111111-1111-4111-8111-111111111111"

	if err := order.StartSaga(sagaID, StatusPendingAuthorization, nil); err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	if order.SagaStatus() != SagaStatusAwaitingInventory || order.CurrentSagaID() == nil {
		t.Fatalf("saga was not started")
	}
	if !order.CanProcessSaga(sagaID) {
		t.Fatalf("expected current saga to be processable")
	}
	if order.CanProcessSaga("22222222-2222-4222-8222-222222222222") {
		t.Fatalf("unexpected saga_id should be rejected")
	}

	order.CompleteSaga()
	if order.SagaStatus() != SagaStatusIdle || order.CurrentSagaID() != nil || order.SagaTargetStatus() != nil {
		t.Fatalf("saga was not cleared")
	}

	if err := order.StartSaga(sagaID, StatusCompleted, nil); err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	order.FailSaga()
	if order.SagaStatus() != SagaStatusFailed || order.CanProcessSaga(sagaID) {
		t.Fatalf("failed saga should not process duplicate events")
	}
}

func TestOrderStatusAllowsCompensatingCancelAfterCompletedAndPaid(t *testing.T) {
	if !StatusCompleted.CanTransitionTo(StatusCanceled) {
		t.Fatalf("COMPLETED should allow compensating cancel")
	}
	if !StatusPaid.CanTransitionTo(StatusCanceled) {
		t.Fatalf("PAID should allow compensating cancel")
	}
	if StatusDelivered.CanTransitionTo(StatusCanceled) {
		t.Fatalf("DELIVERED should remain final")
	}
}

func TestOrderStatusPaymentTransitions(t *testing.T) {
	if !StatusCompleted.CanTransitionTo(StatusAwaitingPayment) {
		t.Fatalf("COMPLETED should advance to AWAITING_PAYMENT")
	}
	if StatusCompleted.CanTransitionTo(StatusPaid) {
		t.Fatalf("COMPLETED should not skip AWAITING_PAYMENT")
	}
	if !StatusAwaitingPayment.CanTransitionTo(StatusPaid) {
		t.Fatalf("AWAITING_PAYMENT should allow payment approval")
	}
	if !StatusAwaitingPayment.CanTransitionTo(StatusCompleted) {
		t.Fatalf("AWAITING_PAYMENT should allow retry after rejected payment")
	}
	if !StatusAwaitingPayment.CanTransitionTo(StatusCanceled) {
		t.Fatalf("AWAITING_PAYMENT should allow cancel")
	}
}
