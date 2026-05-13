package email

import "fmt"

// MockEmailService implementação mock para testes
type MockEmailService struct {
	SentEmails []SentEmail
	ShouldFail bool
}

type SentEmail struct {
	CustomerEmail   string
	CustomerName    string
	ServiceOrderID  string
	OldStatus       string
	NewStatus       string
}

// NewMockEmailService cria uma nova instância do mock
func NewMockEmailService() *MockEmailService {
	return &MockEmailService{
		SentEmails: []SentEmail{},
		ShouldFail: false,
	}
}

// SendStatusUpdateEmail implementação mock
func (m *MockEmailService) SendStatusUpdateEmail(customerEmail, customerName, serviceOrderID, oldStatus, newStatus string) error {
	if m.ShouldFail {
		return fmt.Errorf("mock email service error")
	}

	m.SentEmails = append(m.SentEmails, SentEmail{
		CustomerEmail:  customerEmail,
		CustomerName:   customerName,
		ServiceOrderID: serviceOrderID,
		OldStatus:      oldStatus,
		NewStatus:      newStatus,
	})

	return nil
}

// Reset limpa os emails enviados
func (m *MockEmailService) Reset() {
	m.SentEmails = []SentEmail{}
	m.ShouldFail = false
}
