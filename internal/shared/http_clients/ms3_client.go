package http_clients

import (
	"context"
	"os"

	"oficina-tech/internal/shared/dto"
)

type MS3Client struct {
	rest restClient
}

func NewMS3Client() *MS3Client {
	baseURL := os.Getenv("MS3_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8083"
	}
	return &MS3Client{rest: newRESTClient(baseURL)}
}

func NewMS3ClientWithBaseURL(baseURL string) *MS3Client {
	return &MS3Client{rest: newRESTClient(baseURL)}
}

func (c *MS3Client) GetProductByID(ctx context.Context, id string) (*dto.ProductDTO, error) {
	var product dto.ProductDTO
	if err := c.rest.get(ctx, "/products/"+id, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (c *MS3Client) GetServiceByID(ctx context.Context, id string) (*dto.ServiceDTO, error) {
	var service dto.ServiceDTO
	if err := c.rest.get(ctx, "/services/"+id, &service); err != nil {
		return nil, err
	}
	return &service, nil
}
