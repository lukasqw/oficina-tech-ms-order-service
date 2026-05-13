package service_order

import (
	"time"
)

// ItemType representa o tipo de item (produto ou serviço)
type ItemType string

const (
	ItemTypeProduct ItemType = "PRODUCT"
	ItemTypeService ItemType = "SERVICE"
)

// IsValid verifica se o tipo de item é válido
func (t ItemType) IsValid() bool {
	return t == ItemTypeProduct || t == ItemTypeService
}

// ServiceOrderItem representa um item individual em uma ordem de serviço
type ServiceOrderItem struct {
	id             string
	serviceOrderID string
	historyID      *string // ID do histórico associado quando o item é substituído
	itemType       ItemType
	referenceID    string // ID do produto ou serviço
	name           string // Nome capturado no momento da adição
	quantity       int
	unitPrice      int // Preço em centavos
	createdAt      time.Time
	updatedAt      time.Time
	deletedAt      *time.Time
}

// NewServiceOrderItem cria um novo item de ordem de serviço com validação
// serviceOrderID pode ser vazio se o item for criado antes da ordem (será definido posteriormente)
func NewServiceOrderItem(
	serviceOrderID string,
	itemType ItemType,
	referenceID string,
	name string,
	quantity int,
	unitPrice int,
) (*ServiceOrderItem, error) {
	if !itemType.IsValid() {
		return nil, ErrInvalidItemType
	}
	if referenceID == "" {
		return nil, ErrInvalidReferenceID
	}
	if name == "" {
		return nil, ErrInvalidItemName
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if unitPrice <= 0 {
		return nil, ErrInvalidUnitPrice
	}

	now := time.Now()
	return &ServiceOrderItem{
		serviceOrderID: serviceOrderID,
		itemType:       itemType,
		referenceID:    referenceID,
		name:           name,
		quantity:       quantity,
		unitPrice:      unitPrice,
		createdAt:      now,
		updatedAt:      now,
	}, nil
}

// ReconstructServiceOrderItem reconstrói um item da persistência sem validação
func ReconstructServiceOrderItem(
	id string,
	serviceOrderID string,
	historyID *string,
	itemType ItemType,
	referenceID string,
	name string,
	quantity int,
	unitPrice int,
	createdAt time.Time,
	updatedAt time.Time,
	deletedAt *time.Time,
) *ServiceOrderItem {
	return &ServiceOrderItem{
		id:             id,
		serviceOrderID: serviceOrderID,
		historyID:      historyID,
		itemType:       itemType,
		referenceID:    referenceID,
		name:           name,
		quantity:       quantity,
		unitPrice:      unitPrice,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
		deletedAt:      deletedAt,
	}
}

// UpdateQuantity atualiza a quantidade do item com validação
func (i *ServiceOrderItem) UpdateQuantity(quantity int) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	i.quantity = quantity
	i.updatedAt = time.Now()
	return nil
}

// Subtotal calcula o subtotal do item (quantity × unitPrice)
func (i *ServiceOrderItem) Subtotal() int {
	return i.quantity * i.unitPrice
}

// MarkAsDeleted realiza soft delete do item
func (i *ServiceOrderItem) MarkAsDeleted() {
	now := time.Now()
	i.deletedAt = &now
	i.updatedAt = now
}

// IsDeleted verifica se o item está deletado
func (i *ServiceOrderItem) IsDeleted() bool {
	return i.deletedAt != nil
}

// SetID define o ID do item (usado após persistência)
func (i *ServiceOrderItem) SetID(id string) error {
	if id == "" {
		return ErrInvalidItemID
	}
	i.id = id
	return nil
}

// SetServiceOrderID define o ID da ordem de serviço (usado ao criar itens antes da ordem)
func (i *ServiceOrderItem) SetServiceOrderID(serviceOrderID string) error {
	if serviceOrderID == "" {
		return ErrInvalidServiceOrderID
	}
	i.serviceOrderID = serviceOrderID
	return nil
}

// SetHistoryID associa o item a um registro de histórico
func (i *ServiceOrderItem) SetHistoryID(historyID string) error {
	if historyID == "" {
		return ErrInvalidHistoryID
	}
	i.historyID = &historyID
	i.updatedAt = time.Now()
	return nil
}

// Getters

func (i *ServiceOrderItem) ID() string {
	return i.id
}

func (i *ServiceOrderItem) ServiceOrderID() string {
	return i.serviceOrderID
}

func (i *ServiceOrderItem) ItemType() ItemType {
	return i.itemType
}

func (i *ServiceOrderItem) ReferenceID() string {
	return i.referenceID
}

func (i *ServiceOrderItem) Name() string {
	return i.name
}

func (i *ServiceOrderItem) Quantity() int {
	return i.quantity
}

func (i *ServiceOrderItem) UnitPrice() int {
	return i.unitPrice
}

func (i *ServiceOrderItem) CreatedAt() time.Time {
	return i.createdAt
}

func (i *ServiceOrderItem) UpdatedAt() time.Time {
	return i.updatedAt
}

func (i *ServiceOrderItem) DeletedAt() *time.Time {
	return i.deletedAt
}

func (i *ServiceOrderItem) HistoryID() *string {
	return i.historyID
}
