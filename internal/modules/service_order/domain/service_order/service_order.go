package service_order

import (
	"time"
)

type ServiceOrder struct {
	id               string
	customerID       string
	vehicleID        string
	description      string
	status           OrderStatus
	sagaStatus       string
	currentSagaID    *string
	sagaTargetStatus *OrderStatus
	sagaNotes        *string
	mpPreferenceID   *string
	mpPaymentID      *string
	paymentURL       *string
	customerEmail    string
	customerName     string
	items            []*ServiceOrderItem
	closedAt         *time.Time
	createdAt        time.Time
	updatedAt        time.Time
	deletedAt        *time.Time
}

const (
	SagaStatusIdle              = "IDLE"
	SagaStatusAwaitingInventory = "AWAITING_INVENTORY"
	SagaStatusAwaitingPayment   = "AWAITING_PAYMENT"
	SagaStatusFailed            = "FAILED"
)

// NewServiceOrder cria uma nova ordem de serviço com status RECEIVED
func NewServiceOrder(customerID, vehicleID, description string) (*ServiceOrder, error) {
	if customerID == "" {
		return nil, ErrInvalidCustomerID
	}
	if vehicleID == "" {
		return nil, ErrInvalidVehicleID
	}

	now := time.Now()
	return &ServiceOrder{
		customerID:  customerID,
		vehicleID:   vehicleID,
		description: description,
		status:      StatusReceived,
		sagaStatus:  SagaStatusIdle,
		items:       []*ServiceOrderItem{},
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// ReconstructServiceOrder reconstrói uma ordem de serviço da persistência
func ReconstructServiceOrder(
	id, customerID, vehicleID, description string,
	status OrderStatus,
	sagaStatus string,
	currentSagaID *string,
	sagaTargetStatus *OrderStatus,
	sagaNotes *string,
	mpPreferenceID *string,
	mpPaymentID *string,
	paymentURL *string,
	items []*ServiceOrderItem,
	closedAt *time.Time,
	createdAt, updatedAt time.Time,
	deletedAt *time.Time,
) (*ServiceOrder, error) {
	if id == "" {
		return nil, ErrInvalidServiceOrderID
	}
	if customerID == "" {
		return nil, ErrInvalidCustomerID
	}
	if vehicleID == "" {
		return nil, ErrInvalidVehicleID
	}
	if !status.IsValid() {
		return nil, ErrInvalidStatus
	}

	if items == nil {
		items = []*ServiceOrderItem{}
	}
	if sagaStatus == "" {
		sagaStatus = SagaStatusIdle
	}

	return &ServiceOrder{
		id:               id,
		customerID:       customerID,
		vehicleID:        vehicleID,
		description:      description,
		status:           status,
		sagaStatus:       sagaStatus,
		currentSagaID:    currentSagaID,
		sagaTargetStatus: sagaTargetStatus,
		sagaNotes:        sagaNotes,
		mpPreferenceID:   mpPreferenceID,
		mpPaymentID:      mpPaymentID,
		paymentURL:       paymentURL,
		items:            items,
		closedAt:         closedAt,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
		deletedAt:        deletedAt,
	}, nil
}

// UpdateStatus atualiza o status da ordem e gerencia closedAt
func (s *ServiceOrder) UpdateStatus(newStatus OrderStatus) error {
	if !newStatus.IsValid() {
		return ErrInvalidStatus
	}

	if !s.status.CanTransitionTo(newStatus) {
		return ErrInvalidStatusTransition
	}

	s.status = newStatus
	s.updatedAt = time.Now()

	// Registra closedAt quando status muda para um estado final
	if (newStatus == StatusCompleted || newStatus == StatusPaid || newStatus == StatusDelivered ||
		newStatus == StatusCanceled || newStatus == StatusAuthorizationDenied) &&
		s.closedAt == nil {
		now := time.Now()
		s.closedAt = &now
	}

	return nil
}

// UpdateCustomer atualiza o cliente associado
func (s *ServiceOrder) UpdateCustomer(customerID string) error {
	if customerID == "" {
		return ErrInvalidCustomerID
	}

	s.customerID = customerID
	s.updatedAt = time.Now()
	return nil
}

// UpdateVehicle atualiza o veículo associado
func (s *ServiceOrder) UpdateVehicle(vehicleID string) error {
	if vehicleID == "" {
		return ErrInvalidVehicleID
	}

	s.vehicleID = vehicleID
	s.updatedAt = time.Now()
	return nil
}

// UpdateDescription atualiza a descrição da ordem
func (s *ServiceOrder) UpdateDescription(description string) {
	s.description = description
	s.updatedAt = time.Now()
}

// GetProductItems retorna apenas os itens do tipo PRODUCT que não estão deletados
func (s *ServiceOrder) GetProductItems() []*ServiceOrderItem {
	var productItems []*ServiceOrderItem
	for _, item := range s.items {
		if !item.IsDeleted() && item.ItemType() == ItemTypeProduct {
			productItems = append(productItems, item)
		}
	}
	return productItems
}

// MarkAsDeleted realiza soft delete
func (s *ServiceOrder) MarkAsDeleted() {
	now := time.Now()
	s.deletedAt = &now
	s.updatedAt = now
}

func (s *ServiceOrder) StartSaga(sagaID string, targetStatus OrderStatus, notes *string) error {
	if sagaID == "" {
		return ErrInvalidSagaID
	}
	if !targetStatus.IsValid() {
		return ErrInvalidStatus
	}

	s.sagaStatus = SagaStatusAwaitingInventory
	s.currentSagaID = &sagaID
	s.sagaTargetStatus = &targetStatus
	s.sagaNotes = notes
	s.updatedAt = time.Now()
	return nil
}

func (s *ServiceOrder) CompleteSaga() {
	s.sagaStatus = SagaStatusIdle
	s.currentSagaID = nil
	s.sagaTargetStatus = nil
	s.sagaNotes = nil
	s.updatedAt = time.Now()
}

func (s *ServiceOrder) AwaitPayment(preferenceID, paymentURL string) error {
	if preferenceID == "" {
		return ErrInvalidPaymentPreference
	}
	if paymentURL == "" {
		return ErrInvalidPaymentURL
	}
	if err := s.UpdateStatus(StatusAwaitingPayment); err != nil {
		return err
	}
	s.mpPreferenceID = &preferenceID
	s.paymentURL = &paymentURL
	s.sagaStatus = SagaStatusAwaitingPayment
	s.currentSagaID = nil
	s.sagaTargetStatus = nil
	s.sagaNotes = nil
	s.updatedAt = time.Now()
	return nil
}

func (s *ServiceOrder) ConfirmPayment(paymentID string) error {
	if paymentID == "" {
		return ErrInvalidPaymentID
	}
	if err := s.UpdateStatus(StatusPaid); err != nil {
		return err
	}
	s.mpPaymentID = &paymentID
	s.sagaStatus = SagaStatusIdle
	s.currentSagaID = nil
	s.sagaTargetStatus = nil
	s.sagaNotes = nil
	s.updatedAt = time.Now()
	return nil
}

func (s *ServiceOrder) RejectPayment() error {
	if err := s.UpdateStatus(StatusPaymentRejected); err != nil {
		return err
	}
	s.sagaStatus = SagaStatusIdle
	s.currentSagaID = nil
	s.sagaTargetStatus = nil
	s.sagaNotes = nil
	s.updatedAt = time.Now()
	return nil
}

// CancelAfterRefund transiciona a OS de PAID para CANCELED quando o estorno
// foi realizado externamente (ex: painel do vendedor no Mercado Pago).
// O webhook de "refunded" do MP dispara esta transição.
func (s *ServiceOrder) CancelAfterRefund() error {
	if s.status != StatusPaid {
		return ErrInvalidStatusTransition
	}
	return s.UpdateStatus(StatusCanceled)
}

func (s *ServiceOrder) FailSaga() {
	s.sagaStatus = SagaStatusFailed
	s.currentSagaID = nil
	s.sagaTargetStatus = nil
	s.sagaNotes = nil
	s.updatedAt = time.Now()
}

func (s *ServiceOrder) CanProcessSaga(sagaID string) bool {
	return s.sagaStatus == SagaStatusAwaitingInventory &&
		s.currentSagaID != nil &&
		*s.currentSagaID == sagaID
}

// IsDeleted verifica se a ordem está deletada
func (s *ServiceOrder) IsDeleted() bool {
	return s.deletedAt != nil
}

// IsClosed verifica se a ordem está fechada (PAID, DELIVERED, CANCELED ou AUTHORIZATION_DENIED)
func (s *ServiceOrder) IsClosed() bool {
	return s.status == StatusPaid ||
		s.status == StatusDelivered ||
		s.status == StatusCanceled ||
		s.status == StatusAuthorizationDenied
}

// CanModifyItems verifica se os itens da ordem podem ser modificados
// Itens não podem ser modificados no status PENDING_AUTHORIZATION e pos
func (s *ServiceOrder) CanModifyItems() bool {
	return s.status == StatusReceived || s.status == StatusDiagnosing
}

// SetID define o ID da ordem (usado após persistência)
func (s *ServiceOrder) SetID(id string) error {
	if id == "" {
		return ErrInvalidServiceOrderID
	}
	s.id = id
	return nil
}

// Item Management Methods

// AddItem adiciona um item à ordem de serviço
// Se já existir um item com o mesmo referenceID e itemType, incrementa a quantidade
func (s *ServiceOrder) AddItem(item *ServiceOrderItem) error {
	if item == nil {
		return ErrInvalidItemID
	}

	if s.IsClosed() {
		return ErrCannotModifyClosedOrder
	}

	if !s.CanModifyItems() {
		return ErrCannotModifyItemsAfterPending
	}

	// Verifica se já existe um item com o mesmo referenceID e itemType (não deletado)
	for _, existingItem := range s.items {
		if !existingItem.IsDeleted() &&
			existingItem.ReferenceID() == item.ReferenceID() &&
			existingItem.ItemType() == item.ItemType() {
			// Incrementa a quantidade do item existente
			newQuantity := existingItem.Quantity() + item.Quantity()
			if err := existingItem.UpdateQuantity(newQuantity); err != nil {
				return err
			}
			s.updatedAt = time.Now()
			return nil
		}
	}

	// Se não encontrou item duplicado, adiciona novo item
	s.items = append(s.items, item)
	s.updatedAt = time.Now()
	return nil
}

// RemoveItem remove um item da ordem (soft delete)
func (s *ServiceOrder) RemoveItem(itemID string) error {
	if itemID == "" {
		return ErrInvalidItemID
	}

	if s.IsClosed() {
		return ErrCannotModifyClosedOrder
	}

	if !s.CanModifyItems() {
		return ErrCannotModifyItemsAfterPending
	}

	for _, item := range s.items {
		if item.ID() == itemID && !item.IsDeleted() {
			item.MarkAsDeleted()
			s.updatedAt = time.Now()
			return nil
		}
	}

	return ErrItemNotFound
}

// UpdateItemQuantity atualiza a quantidade de um item específico
func (s *ServiceOrder) UpdateItemQuantity(itemID string, quantity int) error {
	if itemID == "" {
		return ErrInvalidItemID
	}

	if s.IsClosed() {
		return ErrCannotModifyClosedOrder
	}

	if !s.CanModifyItems() {
		return ErrCannotModifyItemsAfterPending
	}

	for _, item := range s.items {
		if item.ID() == itemID && !item.IsDeleted() {
			if err := item.UpdateQuantity(quantity); err != nil {
				return err
			}
			s.updatedAt = time.Now()
			return nil
		}
	}

	return ErrItemNotFound
}

// Items retorna todos os itens da ordem (incluindo deletados)
func (s *ServiceOrder) Items() []*ServiceOrderItem {
	return s.items
}

// TotalAmount calcula o valor total da ordem (soma dos subtotais dos itens não deletados)
func (s *ServiceOrder) TotalAmount() int {
	total := 0
	for _, item := range s.items {
		if !item.IsDeleted() {
			total += item.Subtotal()
		}
	}
	return total
}

// ClearItems remove todos os itens da ordem
func (s *ServiceOrder) ClearItems() {
	s.items = []*ServiceOrderItem{}
	s.updatedAt = time.Now()
}

// Getters

func (s *ServiceOrder) ID() string {
	return s.id
}

func (s *ServiceOrder) CustomerID() string {
	return s.customerID
}

func (s *ServiceOrder) VehicleID() string {
	return s.vehicleID
}

func (s *ServiceOrder) Status() OrderStatus {
	return s.status
}

func (s *ServiceOrder) SagaStatus() string {
	return s.sagaStatus
}

func (s *ServiceOrder) CurrentSagaID() *string {
	return s.currentSagaID
}

func (s *ServiceOrder) SagaTargetStatus() *OrderStatus {
	return s.sagaTargetStatus
}

func (s *ServiceOrder) SagaNotes() *string {
	return s.sagaNotes
}

func (s *ServiceOrder) MPPreferenceID() *string {
	return s.mpPreferenceID
}

// SetCustomerSnapshot persiste email e nome do cliente na OS para uso no pagamento MP.
// Chamado uma vez na criação da OS — evita chamada extra ao MS1 no momento do pagamento.
func (s *ServiceOrder) SetCustomerSnapshot(email, name string) {
	s.customerEmail = email
	s.customerName = name
	s.updatedAt = time.Now()
}

func (s *ServiceOrder) CustomerEmail() string {
	return s.customerEmail
}

func (s *ServiceOrder) CustomerName() string {
	return s.customerName
}

func (s *ServiceOrder) MPPaymentID() *string {
	return s.mpPaymentID
}

func (s *ServiceOrder) PaymentURL() *string {
	return s.paymentURL
}

func (s *ServiceOrder) ClosedAt() *time.Time {
	return s.closedAt
}

func (s *ServiceOrder) CreatedAt() time.Time {
	return s.createdAt
}

func (s *ServiceOrder) UpdatedAt() time.Time {
	return s.updatedAt
}

func (s *ServiceOrder) DeletedAt() *time.Time {
	return s.deletedAt
}

func (s *ServiceOrder) Description() string {
	return s.description
}
