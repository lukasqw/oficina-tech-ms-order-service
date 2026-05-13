package http_clients

import (
	"context"
	"os"

	"oficina-tech/internal/shared/dto"
)

type MS1Client struct {
	rest restClient
}

func NewMS1Client() *MS1Client {
	baseURL := os.Getenv("MS1_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	return &MS1Client{rest: newRESTClient(baseURL)}
}

func NewMS1ClientWithBaseURL(baseURL string) *MS1Client {
	return &MS1Client{rest: newRESTClient(baseURL)}
}

func (c *MS1Client) GetCustomerByID(ctx context.Context, id string) (*dto.CustomerDTO, error) {
	var customer dto.CustomerDTO
	if err := c.rest.get(ctx, "/customers/"+id, &customer); err != nil {
		return nil, err
	}
	return &customer, nil
}

func (c *MS1Client) GetVehicleByID(ctx context.Context, id string) (*dto.VehicleDTO, error) {
	var vehicle dto.VehicleDTO
	if err := c.rest.get(ctx, "/vehicles/"+id, &vehicle); err != nil {
		return nil, err
	}
	return &vehicle, nil
}
