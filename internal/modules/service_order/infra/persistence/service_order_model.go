package persistence

import (
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceOrderModel struct {
	ID                    uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CustomerID            uuid.UUID               `gorm:"type:uuid;not null;index"`
	VehicleID             uuid.UUID               `gorm:"type:uuid;not null;index"`
	Description           string                  `gorm:"type:text"`
	Status                string                  `gorm:"type:varchar(50);not null;index"`
	SagaStatus            string                  `gorm:"type:varchar(50);not null;default:'IDLE';index"`
	CurrentSagaID         *uuid.UUID              `gorm:"type:uuid;index"`
	SagaTargetStatus      *string                 `gorm:"type:varchar(50)"`
	SagaNotes             *string                 `gorm:"type:text"`
	MPOrderID             *string                 `gorm:"column:mp_order_id;type:varchar(255)"`   // migration 003: era mp_preference_id
	MPPaymentID           *string                 `gorm:"column:mp_payment_id;type:varchar(255)"`
	MPOrderStatus         *string                 `gorm:"column:mp_order_status;type:varchar(50)"`
	PaymentURL            *string                 `gorm:"column:payment_url;type:text"`
	PaymentRejectionReason *string                `gorm:"column:payment_rejection_reason;type:varchar(255)"`
	CustomerEmail         *string                 `gorm:"column:customer_email;type:varchar(255)"` // snapshot
	CustomerName          *string                 `gorm:"column:customer_name;type:varchar(255)"`  // snapshot
	ClosedAt              *time.Time              `gorm:"type:timestamp"`
	CreatedAt             time.Time               `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt             time.Time               `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	DeletedAt             gorm.DeletedAt          `gorm:"index"`
	Items                 []ServiceOrderItemModel `gorm:"foreignKey:ServiceOrderID;constraint:OnDelete:CASCADE"`
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

	order, err := service_order.ReconstructServiceOrder(
		m.ID.String(),
		m.CustomerID.String(),
		m.VehicleID.String(),
		m.Description,
		status,
		m.SagaStatus,
		currentSagaID,
		sagaTargetStatus,
		m.SagaNotes,
		m.MPOrderID,
		m.MPPaymentID,
		m.PaymentURL,
		[]*service_order.ServiceOrderItem{},
		m.ClosedAt,
		m.CreatedAt,
		m.UpdatedAt,
		deletedAt,
	)
	if err != nil {
		return nil, err
	}
	if m.CustomerEmail != nil || m.CustomerName != nil {
		email := ""
		name := ""
		if m.CustomerEmail != nil {
			email = *m.CustomerEmail
		}
		if m.CustomerName != nil {
			name = *m.CustomerName
		}
		order.SetCustomerSnapshot(email, name)
	}
	return order, nil
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

	items := make([]*service_order.ServiceOrderItem, 0, len(m.Items))
	for _, itemModel := range m.Items {
		item, err := itemModel.ToDomain()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	order, err := service_order.ReconstructServiceOrder(
		m.ID.String(),
		m.CustomerID.String(),
		m.VehicleID.String(),
		m.Description,
		status,
		m.SagaStatus,
		currentSagaID,
		sagaTargetStatus,
		m.SagaNotes,
		m.MPOrderID,
		m.MPPaymentID,
		m.PaymentURL,
		items,
		m.ClosedAt,
		m.CreatedAt,
		m.UpdatedAt,
		deletedAt,
	)
	if err != nil {
		return nil, err
	}
	if m.CustomerEmail != nil || m.CustomerName != nil {
		email := ""
		name := ""
		if m.CustomerEmail != nil {
			email = *m.CustomerEmail
		}
		if m.CustomerName != nil {
			name = *m.CustomerName
		}
		order.SetCustomerSnapshot(email, name)
	}
	return order, nil
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
		MPOrderID:        order.MPPreferenceID(), // MPPreferenceID() retorna o mp_order_id pós-migration 003
		MPPaymentID:      order.MPPaymentID(),
		PaymentURL:       order.PaymentURL(),
		ClosedAt:         order.ClosedAt(),
		CreatedAt:        order.CreatedAt(),
		UpdatedAt:        order.UpdatedAt(),
	}

	if email := order.CustomerEmail(); email != "" {
		model.CustomerEmail = &email
	}
	if name := order.CustomerName(); name != "" {
		model.CustomerName = &name
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
