package persistence

import (
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceOrderModel struct {
	ID               uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CustomerID       uuid.UUID               `gorm:"type:uuid;not null;index"`
	VehicleID        uuid.UUID               `gorm:"type:uuid;not null;index"`
	Description      string                  `gorm:"type:text"`
	Status           string                  `gorm:"type:varchar(50);not null;index"`
	SagaStatus       string                  `gorm:"type:varchar(50);not null;default:'IDLE';index"`
	CurrentSagaID    *uuid.UUID              `gorm:"type:uuid;index"`
	SagaTargetStatus *string                 `gorm:"type:varchar(50)"`
	SagaNotes        *string                 `gorm:"type:text"`
	MPPreferenceID   *string                 `gorm:"column:mp_preference_id;type:varchar(255)"`
	MPPaymentID      *string                 `gorm:"column:mp_payment_id;type:varchar(255)"`
	PaymentURL       *string                 `gorm:"column:payment_url;type:text"`
	ClosedAt         *time.Time              `gorm:"type:timestamp"`
	CreatedAt        time.Time               `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time               `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	DeletedAt        gorm.DeletedAt          `gorm:"index"`
	Items            []ServiceOrderItemModel `gorm:"foreignKey:ServiceOrderID;constraint:OnDelete:CASCADE"`
}

func (ServiceOrderModel) TableName() string {
	return "service_orders"
}

func (m *ServiceOrderModel) ToDomain() (*service_order.ServiceOrder, error) {
	status, err := service_order.NewOrderStatus(m.Status)
	if err != nil {
		return nil, err
	}

	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}
	currentSagaID := uuidPtrString(m.CurrentSagaID)
	sagaTargetStatus, err := orderStatusPtr(m.SagaTargetStatus)
	if err != nil {
		return nil, err
	}

	// Items will be loaded separately when needed (via FindByIDWithItems)
	items := []*service_order.ServiceOrderItem{}

	return service_order.ReconstructServiceOrder(
		m.ID.String(),
		m.CustomerID.String(),
		m.VehicleID.String(),
		m.Description,
		status,
		m.SagaStatus,
		currentSagaID,
		sagaTargetStatus,
		m.SagaNotes,
		m.MPPreferenceID,
		m.MPPaymentID,
		m.PaymentURL,
		items,
		m.ClosedAt,
		m.CreatedAt,
		m.UpdatedAt,
		deletedAt,
	)
}

func (m *ServiceOrderModel) ToDomainWithItems() (*service_order.ServiceOrder, error) {
	status, err := service_order.NewOrderStatus(m.Status)
	if err != nil {
		return nil, err
	}

	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}
	currentSagaID := uuidPtrString(m.CurrentSagaID)
	sagaTargetStatus, err := orderStatusPtr(m.SagaTargetStatus)
	if err != nil {
		return nil, err
	}

	// Convert items to domain
	items := make([]*service_order.ServiceOrderItem, 0, len(m.Items))
	for _, itemModel := range m.Items {
		item, err := itemModel.ToDomain()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return service_order.ReconstructServiceOrder(
		m.ID.String(),
		m.CustomerID.String(),
		m.VehicleID.String(),
		m.Description,
		status,
		m.SagaStatus,
		currentSagaID,
		sagaTargetStatus,
		m.SagaNotes,
		m.MPPreferenceID,
		m.MPPaymentID,
		m.PaymentURL,
		items,
		m.ClosedAt,
		m.CreatedAt,
		m.UpdatedAt,
		deletedAt,
	)
}

func FromDomain(order *service_order.ServiceOrder) (*ServiceOrderModel, error) {
	var id uuid.UUID
	var err error

	if order.ID() != "" {
		id, err = uuid.Parse(order.ID())
		if err != nil {
			return nil, err
		}
	}

	customerID, err := uuid.Parse(order.CustomerID())
	if err != nil {
		return nil, err
	}

	vehicleID, err := uuid.Parse(order.VehicleID())
	if err != nil {
		return nil, err
	}

	model := &ServiceOrderModel{
		ID:               id,
		CustomerID:       customerID,
		VehicleID:        vehicleID,
		Description:      order.Description(),
		Status:           order.Status().String(),
		SagaStatus:       order.SagaStatus(),
		CurrentSagaID:    parseUUIDPtr(order.CurrentSagaID()),
		SagaTargetStatus: orderStatusStringPtr(order.SagaTargetStatus()),
		SagaNotes:        order.SagaNotes(),
		MPPreferenceID:   order.MPPreferenceID(),
		MPPaymentID:      order.MPPaymentID(),
		PaymentURL:       order.PaymentURL(),
		ClosedAt:         order.ClosedAt(),
		CreatedAt:        order.CreatedAt(),
		UpdatedAt:        order.UpdatedAt(),
	}

	if order.DeletedAt() != nil {
		model.DeletedAt = gorm.DeletedAt{
			Time:  *order.DeletedAt(),
			Valid: true,
		}
	}

	return model, nil
}

func uuidPtrString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	s := value.String()
	return &s
}

func parseUUIDPtr(value *string) *uuid.UUID {
	if value == nil || *value == "" {
		return nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil
	}
	return &parsed
}

func orderStatusPtr(value *string) (*service_order.OrderStatus, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	status, err := service_order.NewOrderStatus(*value)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func orderStatusStringPtr(value *service_order.OrderStatus) *string {
	if value == nil {
		return nil
	}
	s := value.String()
	return &s
}
