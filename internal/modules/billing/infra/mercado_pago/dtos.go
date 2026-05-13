package mercado_pago

type preferenceRequest struct {
	Items             []preferenceItemRequest `json:"items"`
	ExternalReference string                  `json:"external_reference"`
	NotificationURL   string                  `json:"notification_url,omitempty"`
}

type preferenceItemRequest struct {
	Title     string  `json:"title"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type preferenceResponse struct {
	ID               string `json:"id"`
	InitPoint        string `json:"init_point"`
	SandboxInitPoint string `json:"sandbox_init_point"`
}

type paymentResponse struct {
	ID                any    `json:"id"`
	Status            string `json:"status"`
	ExternalReference string `json:"external_reference"`
}
