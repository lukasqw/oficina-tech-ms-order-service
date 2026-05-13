package dto

// ProductDTO is a shared data transfer object for product information
// Used for inter-module communication
type ProductDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	ProductType string `json:"product_type"`
}
