package service_order

import "context"

type RepositoryFilters struct {
	CustomerID    *string
	Status        *OrderStatus
	SortByStatus  bool
	HideCompleted bool
}

type Repository interface {
	Save(ctx context.Context, order *ServiceOrder) error
	SaveWithItems(ctx context.Context, order *ServiceOrder) error
	FindByID(ctx context.Context, id string) (*ServiceOrder, error)
	FindByIDWithItems(ctx context.Context, id string) (*ServiceOrder, error)
	FindAll(ctx context.Context) ([]*ServiceOrder, error)
	FindAllWithFilters(ctx context.Context, filters RepositoryFilters) ([]*ServiceOrder, error)
	FindByCustomerID(ctx context.Context, customerID string) ([]*ServiceOrder, error)
	FindByStatus(ctx context.Context, status OrderStatus) ([]*ServiceOrder, error)
	FindBySagaStatus(ctx context.Context, sagaStatus string) ([]*ServiceOrder, error)
	Delete(ctx context.Context, id string) error
	UpdateItemsHistoryID(ctx context.Context, itemIDs []string, historyID string) error
}
