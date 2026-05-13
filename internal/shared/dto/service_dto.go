package dto

// ServiceDTO is a shared data transfer object for service information
// Used for inter-module communication
type ServiceDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
}
