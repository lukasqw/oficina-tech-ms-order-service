package handlers

import (
	"encoding/json"
	"net/http"

	"oficina-tech/internal/modules/service_order/application/usecases"
	"oficina-tech/internal/modules/service_order/infra/http/dto"
	"oficina-tech/internal/shared/infra/http/middleware"
	"oficina-tech/internal/shared/infra/observability"
	"oficina-tech/internal/shared/utils"
	"oficina-tech/internal/shared/validators"
)

// ServiceOrderHandler gerencia as requisições HTTP para ordens de serviço
type ServiceOrderHandler struct {
	createUseCase        *usecases.CreateServiceOrder
	getUseCase           *usecases.GetServiceOrder
	getAllUseCase        *usecases.GetAllServiceOrders
	updateUseCase        *usecases.UpdateServiceOrder
	deleteUseCase        *usecases.DeleteServiceOrder
	advanceStatusUseCase *usecases.AdvanceServiceOrderStatus
	authorizeUseCase     *usecases.RespondToAuthorization
	getHistoryUseCase    *usecases.GetServiceOrderHistory
}

// NewServiceOrderHandler cria uma nova instância do handler com injeção de use cases
func NewServiceOrderHandler(
	createUseCase *usecases.CreateServiceOrder,
	getUseCase *usecases.GetServiceOrder,
	getAllUseCase *usecases.GetAllServiceOrders,
	updateUseCase *usecases.UpdateServiceOrder,
	deleteUseCase *usecases.DeleteServiceOrder,
	advanceStatusUseCase *usecases.AdvanceServiceOrderStatus,
	authorizeUseCase *usecases.RespondToAuthorization,
	getHistoryUseCase *usecases.GetServiceOrderHistory,
) *ServiceOrderHandler {
	return &ServiceOrderHandler{
		createUseCase:        createUseCase,
		getUseCase:           getUseCase,
		getAllUseCase:        getAllUseCase,
		updateUseCase:        updateUseCase,
		deleteUseCase:        deleteUseCase,
		advanceStatusUseCase: advanceStatusUseCase,
		authorizeUseCase:     authorizeUseCase,
		getHistoryUseCase:    getHistoryUseCase,
	}
}

// CreateServiceOrder godoc
// @Summary      Create a new service order
// @Description  Creates a new service order with items (products and/or services) for a customer's vehicle
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        request  body      dto.CreateServiceOrderRequest  true  "Service order creation data"
// @Success      201      {object}  utils.Envelope{data=dto.ServiceOrderResponse}
// @Failure      400      {object}  utils.Envelope  "Invalid request body or validation failed"
// @Failure      401      {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404      {object}  utils.Envelope  "Customer or vehicle not found"
// @Failure      500      {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders [post]
func (h *ServiceOrderHandler) CreateServiceOrder(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.create")
	defer span.End()

	var req dto.CreateServiceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Validar request usando validator
	if err := validators.ValidateStruct(&req); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeValidationFailed, err.Error())
		return
	}

	// Converter items do request para input do use case
	items := make([]usecases.ItemInput, len(req.Items))
	for i, item := range req.Items {
		items[i] = usecases.ItemInput{
			ItemType:    item.ItemType,
			ReferenceID: item.ReferenceID,
			Quantity:    item.Quantity,
		}
	}

	// Executar use case
	input := usecases.CreateServiceOrderInput{
		CustomerID:  req.CustomerID,
		VehicleID:   req.VehicleID,
		Description: req.Description,
		Items:       items,
	}

	output, err := h.createUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	// Converter items para response DTO
	responseItems := make([]dto.ServiceOrderItemResponse, len(output.Items))
	for i, item := range output.Items {
		responseItems[i] = dto.ServiceOrderItemResponse{
			ID:          item.ID,
			ItemType:    item.ItemType,
			ReferenceID: item.ReferenceID,
			Name:        item.Name,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Subtotal:    item.Subtotal,
			CreatedAt:   utils.FormatTimeRFC3339(item.CreatedAt),
		}
	}

	// Calcular total amount
	totalAmount := 0
	for _, item := range output.Items {
		totalAmount += item.Subtotal
	}

	// Converter output para response DTO
	response := dto.ServiceOrderResponse{
		ID:          output.ID,
		CustomerID:  output.CustomerID,
		VehicleID:   output.VehicleID,
		Description: output.Description,
		Status:      output.Status,
		SagaStatus:  output.SagaStatus,
		Items:       responseItems,
		TotalAmount: totalAmount,
		CreatedAt:   utils.FormatTimeRFC3339(output.CreatedAt),
		UpdatedAt:   utils.FormatTimeRFC3339(output.UpdatedAt),
	}

	// Usar utils.RespondJSON para resposta
	utils.RespondSuccess(w, http.StatusCreated, response)
}

// GetServiceOrder godoc
// @Summary      Get service order by ID
// @Description  Retrieves a service order with complete details including customer, vehicle, and items
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Service Order UUID"
// @Success      200  {object}  utils.Envelope{data=dto.ServiceOrderResponse}
// @Failure      400  {object}  utils.Envelope  "Invalid UUID format"
// @Failure      401  {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404  {object}  utils.Envelope  "Service order not found"
// @Failure      500  {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders/{id} [get]
func (h *ServiceOrderHandler) GetServiceOrder(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.get")
	defer span.End()

	id := r.PathValue("id")

	// Validar UUID
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	// Executar use case
	input := usecases.GetServiceOrderInput{
		ID: id,
	}

	output, err := h.getUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	// Converter items para response DTO
	items := make([]dto.ServiceOrderItemResponse, len(output.Items))
	for i, item := range output.Items {
		items[i] = dto.ServiceOrderItemResponse{
			ID:          item.ID,
			ItemType:    item.ItemType,
			ReferenceID: item.ReferenceID,
			Name:        item.Name,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Subtotal:    item.Subtotal,
			CreatedAt:   utils.FormatTimeRFC3339(item.CreatedAt),
		}
	}

	// Converter output para response DTO
	response := dto.ServiceOrderResponse{
		ID:          output.ID,
		CustomerID:  output.CustomerID,
		VehicleID:   output.VehicleID,
		Description: output.Description,
		Status:      output.Status,
		SagaStatus:  output.SagaStatus,
		Items:       items,
		TotalAmount: output.TotalAmount,
		CreatedAt:   utils.FormatTimeRFC3339(output.CreatedAt),
		UpdatedAt:   utils.FormatTimeRFC3339(output.UpdatedAt),
	}

	// Adicionar dados do cliente se disponível
	if output.Customer != nil {
		response.Customer = &dto.CustomerResponse{
			ID:    output.Customer.ID,
			Name:  output.Customer.Name,
			Email: output.Customer.Email,
			Phone: output.Customer.Phone,
		}
	}

	// Adicionar dados do veículo se disponível
	if output.Vehicle != nil {
		response.Vehicle = &dto.VehicleResponse{
			ID:              output.Vehicle.ID,
			CustomerID:      output.Vehicle.CustomerID,
			LicensePlate:    output.Vehicle.LicensePlate,
			Brand:           output.Vehicle.Brand,
			Model:           output.Vehicle.Model,
			ModelYear:       output.Vehicle.ModelYear,
			ManufactureYear: output.Vehicle.ManufactureYear,
		}
	}

	// Formatar ClosedAt se existir
	if output.ClosedAt != nil {
		closedAt := utils.FormatTimeRFC3339(*output.ClosedAt)
		response.ClosedAt = &closedAt
	}

	utils.RespondSuccess(w, http.StatusOK, response)
}

// GetAllServiceOrders godoc
// @Summary      List all service orders
// @Description  Retrieves all service orders with optional filtering by customer ID, status, and sorting options
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        customer_id     query     string  false  "Filter by customer UUID"
// @Param        status          query     string  false  "Filter by order status" Enums(RECEIVED, DIAGNOSING, PENDING_AUTHORIZATION, AUTHORIZED, IN_PROGRESS, COMPLETED, PAID, DELIVERED, CANCELED, AUTHORIZATION_DENIED)
// @Param        sort_by_status  query     string  false  "Sort by status workflow order (true/false, default: false). When true, orders oldest first"
// @Param        hide_completed  query     string  false  "Hide completed orders (COMPLETED, PAID, DELIVERED, AUTHORIZATION_DENIED) (true/false, default: false)"
// @Success      200             {object}  utils.Envelope{data=[]dto.ServiceOrderResponse}
// @Failure      400             {object}  utils.Envelope  "Invalid customer_id format"
// @Failure      401             {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      500             {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders [get]
func (h *ServiceOrderHandler) GetAllServiceOrders(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.get_all")
	defer span.End()

	// Extrair query params
	customerID := r.URL.Query().Get("customer_id")
	status := r.URL.Query().Get("status")
	sortByStatus := r.URL.Query().Get("sort_by_status")
	hideCompleted := r.URL.Query().Get("hide_completed")

	// CUSTOMER role: forçar filtro pelo próprio customer_id, ignorar query param
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	callerID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if callerRole == middleware.RoleCustomer {
		customerID = callerID
	}

	// Preparar input com filtros opcionais
	input := usecases.GetAllServiceOrdersInput{}

	if customerID != "" {
		// Validar UUID do customer_id
		if err := utils.ValidateUUID(customerID); err != nil {
			utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid customer_id format")
			return
		}
		input.CustomerID = &customerID
	}

	if status != "" {
		input.Status = &status
	}

	// Converter query params booleanos
	if sortByStatus == "true" {
		input.SortByStatus = true
	}

	if hideCompleted == "true" {
		input.HideCompleted = true
	}

	// Executar use case
	output, err := h.getAllUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	// Converter output para response DTOs
	responses := make([]dto.ServiceOrderResponse, len(output.Orders))
	for i, order := range output.Orders {
		// Converter items
		items := make([]dto.ServiceOrderItemResponse, len(order.Items))
		for j, item := range order.Items {
			items[j] = dto.ServiceOrderItemResponse{
				ID:          item.ID,
				ItemType:    item.ItemType,
				ReferenceID: item.ReferenceID,
				Name:        item.Name,
				Description: item.Description,
				Quantity:    item.Quantity,
				UnitPrice:   item.UnitPrice,
				Subtotal:    item.Subtotal,
				CreatedAt:   utils.FormatTimeRFC3339(item.CreatedAt),
			}
		}

		responses[i] = dto.ServiceOrderResponse{
			ID:          order.ID,
			CustomerID:  order.CustomerID,
			VehicleID:   order.VehicleID,
			Description: order.Description,
			Status:      order.Status,
			SagaStatus:  order.SagaStatus,
			Items:       items,
			TotalAmount: order.TotalAmount,
			CreatedAt:   utils.FormatTimeRFC3339(order.CreatedAt),
			UpdatedAt:   utils.FormatTimeRFC3339(order.UpdatedAt),
		}

		// Adicionar dados do cliente se disponível
		if order.Customer != nil {
			responses[i].Customer = &dto.CustomerResponse{
				ID:    order.Customer.ID,
				Name:  order.Customer.Name,
				Email: order.Customer.Email,
				Phone: order.Customer.Phone,
			}
		}

		// Adicionar dados do veículo se disponível
		if order.Vehicle != nil {
			responses[i].Vehicle = &dto.VehicleResponse{
				ID:              order.Vehicle.ID,
				CustomerID:      order.Vehicle.CustomerID,
				LicensePlate:    order.Vehicle.LicensePlate,
				Brand:           order.Vehicle.Brand,
				Model:           order.Vehicle.Model,
				ModelYear:       order.Vehicle.ModelYear,
				ManufactureYear: order.Vehicle.ManufactureYear,
			}
		}

		// Formatar ClosedAt se existir
		if order.ClosedAt != nil {
			closedAt := utils.FormatTimeRFC3339(*order.ClosedAt)
			responses[i].ClosedAt = &closedAt
		}
	}

	utils.RespondSuccess(w, http.StatusOK, responses)
}

// UpdateServiceOrder godoc
// @Summary      Update service order
// @Description  Updates a service order's customer, vehicle, status, or items. All fields are optional for partial updates.
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        id       path      string                         true  "Service Order UUID"
// @Param        request  body      dto.UpdateServiceOrderRequest  true  "Service order update data"
// @Success      200      {object}  utils.Envelope{data=dto.ServiceOrderResponse}
// @Failure      400      {object}  utils.Envelope  "Invalid request body or validation failed"
// @Failure      401      {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404      {object}  utils.Envelope  "Service order not found"
// @Failure      409      {object}  utils.Envelope  "Invalid status transition"
// @Failure      500      {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders/{id} [put]
func (h *ServiceOrderHandler) UpdateServiceOrder(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.update")
	defer span.End()

	id := r.PathValue("id")

	// Validar UUID
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	var req dto.UpdateServiceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Validar request usando validator
	if err := validators.ValidateStruct(&req); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeValidationFailed, err.Error())
		return
	}

	// Preparar input do use case
	input := usecases.UpdateServiceOrderInput{
		ID:          id,
		CustomerID:  req.CustomerID,
		VehicleID:   req.VehicleID,
		Description: req.Description,
		Status:      req.Status,
	}

	// Converter items do request para input do use case quando fornecido
	if req.Items != nil {
		items := make([]usecases.ItemInput, len(req.Items))
		for i, item := range req.Items {
			items[i] = usecases.ItemInput{
				ItemType:    item.ItemType,
				ReferenceID: item.ReferenceID,
				Quantity:    item.Quantity,
			}
		}
		input.Items = items
	}

	output, err := h.updateUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	// Converter items para response DTO
	responseItems := make([]dto.ServiceOrderItemResponse, len(output.Items))
	for i, item := range output.Items {
		responseItems[i] = dto.ServiceOrderItemResponse{
			ID:          item.ID,
			ItemType:    item.ItemType,
			ReferenceID: item.ReferenceID,
			Name:        item.Name,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Subtotal:    item.Subtotal,
			CreatedAt:   utils.FormatTimeRFC3339(item.CreatedAt),
		}
	}

	// Calcular total amount
	totalAmount := 0
	for _, item := range output.Items {
		totalAmount += item.Subtotal
	}

	// Converter output para response DTO
	response := dto.ServiceOrderResponse{
		ID:          output.ID,
		CustomerID:  output.CustomerID,
		VehicleID:   output.VehicleID,
		Description: output.Description,
		Status:      output.Status,
		SagaStatus:  output.SagaStatus,
		Items:       responseItems,
		TotalAmount: totalAmount,
		CreatedAt:   utils.FormatTimeRFC3339(output.CreatedAt),
		UpdatedAt:   utils.FormatTimeRFC3339(output.UpdatedAt),
	}

	// Formatar ClosedAt se existir
	if output.ClosedAt != nil {
		closedAt := utils.FormatTimeRFC3339(*output.ClosedAt)
		response.ClosedAt = &closedAt
	}

	utils.RespondSuccess(w, http.StatusOK, response)
}

// DeleteServiceOrder godoc
// @Summary      Delete service order
// @Description  Soft deletes a service order by ID
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Service Order UUID"
// @Success      200  {object}  utils.Envelope{data=object{message=string}}
// @Failure      400  {object}  utils.Envelope  "Invalid UUID format"
// @Failure      401  {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404  {object}  utils.Envelope  "Service order not found"
// @Failure      500  {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders/{id} [delete]
func (h *ServiceOrderHandler) DeleteServiceOrder(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.delete")
	defer span.End()

	id := r.PathValue("id")

	// Validar UUID
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	// Executar use case
	input := usecases.DeleteServiceOrderInput{
		ID: id,
	}

	output, err := h.deleteUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	if output.Async {
		response := map[string]any{
			"message": "Service order cancellation saga started",
		}
		if output.SagaID != nil {
			response["saga_id"] = *output.SagaID
		}
		utils.RespondSuccess(w, http.StatusAccepted, response)
		return
	}

	utils.RespondSuccess(w, http.StatusOK, map[string]string{"message": "Service order canceled successfully"})
}

// AdvanceServiceOrderStatus godoc
// @Summary      Advance service order to next status
// @Description  Advances a service order to the next status in the workflow. Status transitions follow this sequence: RECEIVED → DIAGNOSING → PENDING_AUTHORIZATION → AUTHORIZED → IN_PROGRESS → COMPLETED → PAID → DELIVERED. Orders can be CANCELED from any non-final status. AUTHORIZATION_DENIED can only be reached from PENDING_AUTHORIZATION.
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Service Order UUID"
// @Success      200  {object}  utils.Envelope{data=dto.ServiceOrderResponse}
// @Failure      400  {object}  utils.Envelope  "Invalid UUID format or invalid status transition"
// @Failure      401  {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404  {object}  utils.Envelope  "Service order not found"
// @Failure      409  {object}  utils.Envelope  "Cannot advance from final status"
// @Failure      500  {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders/{id}/advance [post]
func (h *ServiceOrderHandler) AdvanceServiceOrderStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.advance_status")
	defer span.End()

	id := r.PathValue("id")

	// Validar UUID
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	// Executar use case
	input := usecases.AdvanceServiceOrderStatusInput{
		ServiceOrderID: id,
	}

	output, err := h.advanceStatusUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	if output.Async {
		response := map[string]any{
			"message":     "Service order inventory saga started",
			"status":      output.Status,
			"saga_status": output.SagaStatus,
		}
		if output.SagaID != nil {
			response["saga_id"] = *output.SagaID
		}
		utils.RespondSuccess(w, http.StatusAccepted, response)
		return
	}

	if output.PaymentURL != nil {
		utils.RespondSuccess(w, http.StatusAccepted, map[string]any{
			"message":     "Service order awaiting payment",
			"status":      output.Status,
			"saga_status": output.SagaStatus,
			"payment_url": *output.PaymentURL,
		})
		return
	}

	// Converter output para response DTO
	response := dto.ServiceOrderResponse{
		ID:          output.ID,
		CustomerID:  output.CustomerID,
		VehicleID:   output.VehicleID,
		Description: output.Description,
		Status:      output.Status,
		SagaStatus:  output.SagaStatus,
		CreatedAt:   utils.FormatTimeRFC3339(output.CreatedAt),
		UpdatedAt:   utils.FormatTimeRFC3339(output.UpdatedAt),
	}

	// Formatar ClosedAt se existir
	if output.ClosedAt != nil {
		closedAt := utils.FormatTimeRFC3339(*output.ClosedAt)
		response.ClosedAt = &closedAt
	}

	utils.RespondSuccess(w, http.StatusOK, response)
}

func (h *ServiceOrderHandler) AuthorizeServiceOrder(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.authorize")
	defer span.End()

	id := r.PathValue("id")
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	var req dto.AuthorizeServiceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	callerID, _ := r.Context().Value(middleware.UserIDKey).(string)
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)

	output, err := h.authorizeUseCase.Execute(ctx, usecases.RespondToAuthorizationInput{
		ServiceOrderID: id,
		Approved:       req.Approved,
		Observation:    req.Notes,
		CallerID:       callerID,
		CallerRole:     callerRole,
	})
	if err != nil {
		span.RecordError(err)
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	if output.Async {
		response := map[string]any{
			"message":     "Service order authorization saga started",
			"status":      output.Status,
			"saga_status": output.SagaStatus,
		}
		if output.SagaID != nil {
			response["saga_id"] = *output.SagaID
		}
		utils.RespondSuccess(w, http.StatusAccepted, response)
		return
	}

	response := dto.ServiceOrderResponse{
		ID:          output.ID,
		CustomerID:  output.CustomerID,
		VehicleID:   output.VehicleID,
		Description: output.Description,
		Status:      output.Status,
		SagaStatus:  output.SagaStatus,
		CreatedAt:   utils.FormatTimeRFC3339(output.CreatedAt),
		UpdatedAt:   utils.FormatTimeRFC3339(output.UpdatedAt),
	}
	if output.ClosedAt != nil {
		closedAt := utils.FormatTimeRFC3339(*output.ClosedAt)
		response.ClosedAt = &closedAt
	}
	utils.RespondSuccess(w, http.StatusOK, response)
}

// GetServiceOrderHistory godoc
// @Summary      Get service order history
// @Description  Retrieves the complete audit trail of status changes for a service order, including metadata about each transition
// @Tags         service-orders
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Service Order UUID"
// @Success      200  {object}  utils.Envelope{data=[]dto.HistoryResponse}
// @Failure      400  {object}  utils.Envelope  "Invalid UUID format"
// @Failure      401  {object}  utils.Envelope  "Unauthorized - missing or invalid token"
// @Failure      404  {object}  utils.Envelope  "Service order not found"
// @Failure      500  {object}  utils.Envelope  "Internal server error"
// @Security     BearerAuth
// @Router       /service-orders/{id}/history [get]
func (h *ServiceOrderHandler) GetServiceOrderHistory(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "service_order.get_history")
	defer span.End()

	id := r.PathValue("id")

	// Validar UUID
	if err := utils.ValidateUUID(id); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	// Executar use case
	input := usecases.GetServiceOrderHistoryInput{
		ServiceOrderID: id,
	}

	output, err := h.getHistoryUseCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		// Aplicar error mapping
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	// Converter output para response DTOs
	responses := make([]dto.HistoryResponse, len(output.History))
	for i, history := range output.History {
		responses[i] = dto.HistoryResponse{
			ID:             history.ID,
			ServiceOrderID: history.ServiceOrderID,
			Metadata:       history.Metadata,
			Status:         history.Status,
			CreatedAt:      utils.FormatTimeRFC3339(history.CreatedAt),
		}
	}

	utils.RespondSuccess(w, http.StatusOK, responses)
}
