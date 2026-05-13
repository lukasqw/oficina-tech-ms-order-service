package dto

// CustomerDTO is a shared data transfer object for customer information
// Used for inter-module communication
type CustomerDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}
