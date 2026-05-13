package persistence

import (
	"context"
	"errors"
	"fmt"
	"oficina-tech/internal/modules/service_order/domain/service_order"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HistoryRepositoryImpl implementa o repositório de histórico de ordens de serviço
type HistoryRepositoryImpl struct {
	db *gorm.DB
}

// NewHistoryRepository cria uma nova instância do repositório de histórico
func NewHistoryRepository(db *gorm.DB) service_order.HistoryRepository {
	return &HistoryRepositoryImpl{db: db}
}

// Save persiste um novo registro de histórico
func (r *HistoryRepositoryImpl) Save(ctx context.Context, history *service_order.History) error {
	model, err := FromDomainHistory(history)
	if err != nil {
		return err
	}

	// Se o ID está vazio, gera um novo UUID
	if history.ID() == "" {
		model.ID = uuid.Must(uuid.NewV7())
		if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
			return r.mapGormError(err)
		}
		// Define o ID gerado na entidade de domínio
		if err := history.SetID(model.ID.String()); err != nil {
			return err
		}
	} else {
		// Atualiza registro existente
		if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
			return r.mapGormError(err)
		}
	}

	return nil
}

// FindByServiceOrderID retorna todos os históricos de uma ordem de serviço
// ordenados por created_at DESC (mais recente primeiro)
func (r *HistoryRepositoryImpl) FindByServiceOrderID(ctx context.Context, serviceOrderID string) ([]*service_order.History, error) {
	uid, err := uuid.Parse(serviceOrderID)
	if err != nil {
		return nil, fmt.Errorf("invalid service order UUID: %w", err)
	}

	var models []ServiceOrderHistoryModel
	if err := r.db.WithContext(ctx).Where("service_order_id = ?", uid).
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, r.mapGormError(err)
	}

	// Converte os modelos para entidades de domínio
	histories := make([]*service_order.History, 0, len(models))
	for _, model := range models {
		history, err := model.ToDomain()
		if err != nil {
			return nil, err
		}
		histories = append(histories, history)
	}

	return histories, nil
}

// FindByID retorna um histórico específico pelo ID
func (r *HistoryRepositoryImpl) FindByID(ctx context.Context, id string) (*service_order.History, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid history UUID: %w", err)
	}

	var model ServiceOrderHistoryModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", uid).Error; err != nil {
		return nil, r.mapGormError(err)
	}

	return model.ToDomain()
}

// mapGormError mapeia erros do GORM para erros de domínio
func (r *HistoryRepositoryImpl) mapGormError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return service_order.ErrHistoryNotFound
	}

	return err
}
