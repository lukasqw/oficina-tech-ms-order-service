package persistence

import (
	"encoding/json"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ServiceOrderHistoryModel representa o modelo de persistência para histórico de ordens de serviço
type ServiceOrderHistoryModel struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ServiceOrderID uuid.UUID      `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
	Metadata       datatypes.JSON `gorm:"type:jsonb"` // PostgreSQL JSONB
	Status         string         `gorm:"type:varchar(50);not null"`
	CreatedAt      time.Time      `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP;index:idx_service_order_histories_created_at,sort:desc"`
	UpdatedAt      time.Time      `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

// TableName retorna o nome da tabela no banco de dados
func (ServiceOrderHistoryModel) TableName() string {
	return "service_order_histories"
}

// ToDomain converte o modelo de persistência para a entidade de domínio
func (m *ServiceOrderHistoryModel) ToDomain() (*service_order.History, error) {
	// Deserializa o metadata JSONB para map
	var metadata map[string]interface{}
	if len(m.Metadata) > 0 {
		if err := json.Unmarshal(m.Metadata, &metadata); err != nil {
			return nil, service_order.ErrInvalidMetadata
		}
	} else {
		metadata = make(map[string]interface{})
	}

	// Converte o status string para OrderStatus
	status, err := service_order.NewOrderStatus(m.Status)
	if err != nil {
		return nil, err
	}

	// Reconstrói a entidade de domínio
	return service_order.ReconstructHistory(
		m.ID.String(),
		m.ServiceOrderID.String(),
		metadata,
		status,
		m.CreatedAt,
	), nil
}

// FromDomain converte a entidade de domínio para o modelo de persistência
func FromDomainHistory(history *service_order.History) (*ServiceOrderHistoryModel, error) {
	var id uuid.UUID
	var err error

	// Parse o ID se existir
	if history.ID() != "" {
		id, err = uuid.Parse(history.ID())
		if err != nil {
			return nil, err
		}
	}

	// Parse o ServiceOrderID
	serviceOrderID, err := uuid.Parse(history.ServiceOrderID())
	if err != nil {
		return nil, err
	}

	// Serializa o metadata para JSONB
	metadataJSON, err := json.Marshal(history.Metadata())
	if err != nil {
		return nil, service_order.ErrInvalidMetadata
	}

	return &ServiceOrderHistoryModel{
		ID:             id,
		ServiceOrderID: serviceOrderID,
		Metadata:       datatypes.JSON(metadataJSON),
		Status:         history.Status().String(),
		CreatedAt:      history.CreatedAt(),
	}, nil
}
