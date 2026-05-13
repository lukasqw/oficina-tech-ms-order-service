package service_order

import (
	"context"
	"errors"

	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	"oficina-tech/internal/modules/service_order/application/usecases"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
)

// Public facade errors exposed to other modules
// These generic errors abstract away internal domain error details
var (
	// ErrNotFound indicates that a service order was not found
	// Maps from: service_order.ErrServiceOrderNotFound
	ErrNotFound = errors.New("service order not found")

	// ErrInvalidInput indicates invalid input data or validation failure
	// Maps from: service_order.ErrInvalidServiceOrderID, service_order.ErrInvalidCustomerID,
	// service_order.ErrInvalidVehicleID, and other validation errors
	ErrInvalidInput = errors.New("invalid input")

	// ErrHistoryNotFound indicates that service order history was not found
	// Maps from: service_order.ErrHistoryNotFound
	ErrHistoryNotFound = errors.New("service order history not found")
)

// ServiceOrderModule defines the public interface for the service_order bounded context
// This facade exposes only the necessary operations for inter-module communication
type ServiceOrderModule interface {
	GetServiceOrderByID(ctx context.Context, id string) (*dto.ServiceOrderDTO, error)
	CreateServiceOrder(ctx context.Context, customerID, vehicleID string) (*dto.ServiceOrderDTO, error)
	GetServiceOrdersByCustomerID(ctx context.Context, customerID string) ([]dto.ServiceOrderDTO, error)
	GetServiceOrderHistory(ctx context.Context, serviceOrderID string) ([]dto.HistoryDTO, error)
	GetServiceOrderItems(ctx context.Context, serviceOrderID string) ([]dto.ServiceOrderItemDTO, error)
	RespondToAuthorization(ctx context.Context, serviceOrderID string, approved bool, observation *string) (string, error)
}

type serviceOrderModuleImpl struct {
	getServiceOrderUseCase        *usecases.GetServiceOrder
	createServiceOrderUseCase     *usecases.CreateServiceOrder
	getAllServiceOrdersUseCase    *usecases.GetAllServiceOrders
	getServiceOrderHistoryUseCase *usecases.GetServiceOrderHistory
	respondToAuthorizationUseCase *usecases.RespondToAuthorization
}

// NewServiceOrderModule creates a new instance of ServiceOrderModule
func NewServiceOrderModule(
	repository service_order.Repository,
	historyRepository service_order.HistoryRepository,
	customerAdapter adapters.CustomerAdapter,
	vehicleAdapter adapters.VehicleAdapter,
	productAdapter adapters.ProductAdapter,
	serviceAdapter adapters.ServiceAdapter,
	sagaOrchestrator *appsaga.Orchestrator,
	emailService email.EmailService,
) ServiceOrderModule {
	return &serviceOrderModuleImpl{
		getServiceOrderUseCase:        usecases.NewGetServiceOrder(repository, productAdapter, serviceAdapter, customerAdapter, vehicleAdapter),
		createServiceOrderUseCase:     usecases.NewCreateServiceOrder(repository, customerAdapter, vehicleAdapter, productAdapter, serviceAdapter),
		getAllServiceOrdersUseCase:    usecases.NewGetAllServiceOrders(repository, customerAdapter, vehicleAdapter),
		getServiceOrderHistoryUseCase: usecases.NewGetServiceOrderHistory(historyRepository),
		respondToAuthorizationUseCase: usecases.NewRespondToAuthorization(repository, historyRepository, sagaOrchestrator, customerAdapter, emailService),
	}
}

// GetServiceOrderByID retrieves a service order by ID and returns a simplified DTO
func (m *serviceOrderModuleImpl) GetServiceOrderByID(ctx context.Context, id string) (*dto.ServiceOrderDTO, error) {
	output, err := m.getServiceOrderUseCase.Execute(ctx, usecases.GetServiceOrderInput{ID: id})
	if err != nil {
		// Map domain errors to facade errors
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			return nil, ErrNotFound
		}
		if errors.Is(err, service_order.ErrInvalidServiceOrderID) {
			return nil, ErrInvalidInput
		}
		return nil, err
	}

	return &dto.ServiceOrderDTO{
		ID:         output.ID,
		CustomerID: output.CustomerID,
		VehicleID:  output.VehicleID,
		Status:     output.Status,
		ClosedAt:   output.ClosedAt,
		CreatedAt:  output.CreatedAt,
		UpdatedAt:  output.UpdatedAt,
	}, nil
}

// CreateServiceOrder creates a new service order and returns a simplified DTO
func (m *serviceOrderModuleImpl) CreateServiceOrder(ctx context.Context, customerID, vehicleID string) (*dto.ServiceOrderDTO, error) {
	output, err := m.createServiceOrderUseCase.Execute(ctx, usecases.CreateServiceOrderInput{
		CustomerID: customerID,
		VehicleID:  vehicleID,
	})
	if err != nil {
		return nil, err
	}

	return &dto.ServiceOrderDTO{
		ID:         output.ID,
		CustomerID: output.CustomerID,
		VehicleID:  output.VehicleID,
		Status:     output.Status,
		ClosedAt:   nil, // New orders don't have ClosedAt
		CreatedAt:  output.CreatedAt,
		UpdatedAt:  output.UpdatedAt,
	}, nil
}

// GetServiceOrdersByCustomerID retrieves all service orders for a specific customer
func (m *serviceOrderModuleImpl) GetServiceOrdersByCustomerID(ctx context.Context, customerID string) ([]dto.ServiceOrderDTO, error) {
	output, err := m.getAllServiceOrdersUseCase.Execute(ctx, usecases.GetAllServiceOrdersInput{
		CustomerID: &customerID,
	})
	if err != nil {
		// Map domain errors to facade errors
		if errors.Is(err, service_order.ErrInvalidCustomerID) {
			return nil, ErrInvalidInput
		}
		return nil, err
	}

	// Convert to DTOs
	dtos := make([]dto.ServiceOrderDTO, 0, len(output.Orders))
	for _, order := range output.Orders {
		dtos = append(dtos, dto.ServiceOrderDTO{
			ID:          order.ID,
			CustomerID:  order.CustomerID,
			VehicleID:   order.VehicleID,
			Status:      order.Status,
			TotalAmount: order.TotalAmount,
			ClosedAt:    order.ClosedAt,
			CreatedAt:   order.CreatedAt,
			UpdatedAt:   order.UpdatedAt,
		})
	}

	return dtos, nil
}

// GetServiceOrderHistory retrieves the status history for a specific service order
func (m *serviceOrderModuleImpl) GetServiceOrderHistory(ctx context.Context, serviceOrderID string) ([]dto.HistoryDTO, error) {
	output, err := m.getServiceOrderHistoryUseCase.Execute(ctx, usecases.GetServiceOrderHistoryInput{
		ServiceOrderID: serviceOrderID,
	})
	if err != nil {
		// Map domain errors to facade errors
		if errors.Is(err, service_order.ErrHistoryNotFound) {
			return nil, ErrHistoryNotFound
		}
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			return nil, ErrNotFound
		}
		if errors.Is(err, service_order.ErrInvalidServiceOrderID) {
			return nil, ErrInvalidInput
		}
		return nil, err
	}

	// Convert to DTOs
	dtos := make([]dto.HistoryDTO, 0, len(output.History))
	for _, history := range output.History {
		dtos = append(dtos, dto.HistoryDTO{
			ID:             history.ID,
			ServiceOrderID: history.ServiceOrderID,
			Status:         history.Status,
			Metadata:       history.Metadata,
			CreatedAt:      history.CreatedAt,
		})
	}

	return dtos, nil
}

// GetServiceOrderItems retrieves all items for a specific service order
func (m *serviceOrderModuleImpl) GetServiceOrderItems(ctx context.Context, serviceOrderID string) ([]dto.ServiceOrderItemDTO, error) {
	// Call existing GetServiceOrder use case to retrieve the order with items
	output, err := m.getServiceOrderUseCase.Execute(ctx, usecases.GetServiceOrderInput{ID: serviceOrderID})
	if err != nil {
		// Map domain errors to facade errors
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			return nil, ErrNotFound
		}
		if errors.Is(err, service_order.ErrInvalidServiceOrderID) {
			return nil, ErrInvalidInput
		}
		return nil, err
	}

	// Convert items to DTOs, filtering out deleted items
	dtos := make([]dto.ServiceOrderItemDTO, 0, len(output.Items))
	for _, item := range output.Items {
		// Items are already filtered in the use case, but we ensure no deleted items
		// The GetServiceOrder use case already filters deleted items
		dtos = append(dtos, dto.ServiceOrderItemDTO{
			ID:          item.ID,
			ItemType:    item.ItemType,
			ReferenceID: item.ReferenceID,
			Name:        item.Name,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Subtotal:    item.Subtotal, // Already calculated in the use case
		})
	}

	return dtos, nil
}

// RespondToAuthorization responds to a service order authorization request
func (m *serviceOrderModuleImpl) RespondToAuthorization(ctx context.Context, serviceOrderID string, approved bool, observation *string) (string, error) {
	output, err := m.respondToAuthorizationUseCase.Execute(ctx, usecases.RespondToAuthorizationInput{
		ServiceOrderID: serviceOrderID,
		Approved:       approved,
		Observation:    observation,
	})
	if err != nil {
		// Map domain errors to facade errors
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			return "", ErrNotFound
		}
		if errors.Is(err, service_order.ErrInvalidServiceOrderID) {
			return "", ErrInvalidInput
		}
		if errors.Is(err, service_order.ErrInvalidStatusTransition) {
			return "", ErrInvalidInput
		}
		return "", err
	}

	return output.Status, nil
}
