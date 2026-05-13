package service_order

import "context"

type HistoryRepository interface {
	// Save persiste um novo registro de histórico
	Save(ctx context.Context, history *History) error

	// FindByServiceOrderID retorna todos os históricos de uma ordem de serviço
	// ordenados por created_at DESC (mais recente primeiro)
	FindByServiceOrderID(ctx context.Context, serviceOrderID string) ([]*History, error)

	// FindByID retorna um histórico específico pelo ID
	FindByID(ctx context.Context, id string) (*History, error)
}
