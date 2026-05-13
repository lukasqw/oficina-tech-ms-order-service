package dto

// VehicleDTO is a shared data transfer object for vehicle information
// Used for inter-module communication
type VehicleDTO struct {
	ID              string `json:"id"`
	CustomerID      string `json:"customer_id"`
	LicensePlate    string `json:"license_plate"`
	Brand           string `json:"brand"`
	Model           string `json:"model"`
	ModelYear       int    `json:"model_year"`
	ManufactureYear int    `json:"manufacture_year"`
}
