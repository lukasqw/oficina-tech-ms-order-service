package persistence

import (
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ServiceOrderItemModel representa o modelo de persistência para itens de ordem de serviço
type ServiceOrderItemModel struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ServiceOrderID uuid.UUID      `gorm:"type:uuid;not null;index"`
	HistoryID      *uuid.UUID     `gorm:"type:uuid;index;constraint:OnDelete:SET NULL"` // ID do histórico associado quando o item é substituído
	ItemType       string         `gorm:"type:varchar(20);not null"`
	ReferenceID    uuid.UUID      `gorm:"type:uuid;not null"`
	Name           string         `gorm:"type:varchar(200);not null"`
	Quantity       int            `gorm:"not null"`
	UnitPrice      int            `gorm:"not null"` // Preço em centavos
	CreatedAt      time.Time      `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time      `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// TableName retorna o nome da tabela no banco de dados
func (ServiceOrderItemModel) TableName() string {
	return "service_order_items"
}

// ToDomain converte o modelo de persistência para a entidade de domínio
func (m *ServiceOrderItemModel) ToDomain() (*service_order.ServiceOrderItem, error) {
	itemType := service_order.ItemType(m.ItemType)
	if !itemType.IsValid() {
		return nil, service_order.ErrInvalidItemType
	}

	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}

	var historyID *string
	if m.HistoryID != nil {
		historyIDStr := m.HistoryID.String()
		historyID = &historyIDStr
	}

	return service_order.ReconstructServiceOrderItem(
		m.ID.String(),
		m.ServiceOrderID.String(),
		historyID,
		itemType,
		m.ReferenceID.String(),
		m.Name,
		m.Quantity,
		m.UnitPrice,
		m.CreatedAt,
		m.UpdatedAt,
		deletedAt,
	), nil
}

// FromDomainItem converte a entidade de domínio para o modelo de persistência
func FromDomainItem(item *service_order.ServiceOrderItem) (*ServiceOrderItemModel, error) {
	var id uuid.UUID
	var err error

	if item.ID() != "" {
		id, err = uuid.Parse(item.ID())
		if err != nil {
			return nil, err
		}
	}

	serviceOrderID, err := uuid.Parse(item.ServiceOrderID())
	if err != nil {
		return nil, err
	}

	referenceID, err := uuid.Parse(item.ReferenceID())
	if err != nil {
		return nil, err
	}

	var historyID *uuid.UUID
	if item.HistoryID() != nil {
		parsedHistoryID, err := uuid.Parse(*item.HistoryID())
		if err != nil {
			return nil, err
		}
		historyID = &parsedHistoryID
	}

	model := &ServiceOrderItemModel{
		ID:             id,
		ServiceOrderID: serviceOrderID,
		HistoryID:      historyID,
		ItemType:       string(item.ItemType()),
		ReferenceID:    referenceID,
		Name:           item.Name(),
		Quantity:       item.Quantity(),
		UnitPrice:      item.UnitPrice(),
		CreatedAt:      item.CreatedAt(),
		UpdatedAt:      item.UpdatedAt(),
	}

	if item.DeletedAt() != nil {
		model.DeletedAt = gorm.DeletedAt{
			Time:  *item.DeletedAt(),
			Valid: true,
		}
	}

	return model, nil
}
