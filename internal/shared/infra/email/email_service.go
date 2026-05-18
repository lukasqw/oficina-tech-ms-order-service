package email

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

// EmailService interface para envio de emails
type EmailService interface {
	SendStatusUpdateEmail(customerEmail, customerName, serviceOrderID, oldStatus, newStatus string) error
}

// SMTPEmailService implementação usando SMTP
type SMTPEmailService struct {
	host     string
	port     string
	username string
	password string
	from     string
}

// NewSMTPEmailService cria uma nova instância do serviço de email
func NewSMTPEmailService() *SMTPEmailService {
	return &SMTPEmailService{
		host:     getEnv("SMTP_HOST", "smtp.gmail.com"),
		port:     getEnv("SMTP_PORT", "587"),
		username: getEnv("SMTP_USERNAME", ""),
		password: getEnv("SMTP_PASSWORD", ""),
		from:     getEnv("SMTP_FROM", "noreply@oficinatech.com"),
	}
}

// SendStatusUpdateEmail envia email de atualização de status da OS
func (s *SMTPEmailService) SendStatusUpdateEmail(customerEmail, customerName, serviceOrderID, oldStatus, newStatus string) error {
	// Se não houver configuração de SMTP, apenas loga (modo desenvolvimento)
	if s.username == "" || s.password == "" {
		fmt.Printf("[EMAIL] Status atualizado - OS: %s | Cliente: %s (%s) | %s -> %s\n",
			serviceOrderID, customerName, customerEmail, oldStatus, newStatus)
		return nil
	}

	subject := fmt.Sprintf("Atualização da Ordem de Serviço #%s", serviceOrderID)
	body := s.buildStatusUpdateEmailBody(customerName, serviceOrderID, oldStatus, newStatus)

	return s.sendEmail(customerEmail, subject, body)
}

// buildStatusUpdateEmailBody constrói o corpo do email
func (s *SMTPEmailService) buildStatusUpdateEmailBody(customerName, serviceOrderID, oldStatus, newStatus string) string {
	statusMessages := map[string]string{
		"RECEIVED":              "Recebida - Sua ordem foi registrada em nosso sistema",
		"DIAGNOSING":            "Em Diagnóstico - Estamos avaliando o veículo",
		"PENDING_AUTHORIZATION": "Aguardando Autorização - Orçamento disponível para aprovação",
		"AUTHORIZED":            "Autorizada - Iniciando os serviços",
		"IN_PROGRESS":           "Em Andamento - Serviços sendo executados",
		"COMPLETED":             "Concluída - Serviços finalizados",
		"AWAITING_PAYMENT":      "Aguardando Pagamento - Link de pagamento disponível",
		"READY_FOR_PICKUP":      "Pronta para Retirada - Veículo disponível",
		"PAID":                  "Paga - Pagamento confirmado",
		"DELIVERED":             "Entregue - Veículo retirado",
		"CANCELED":              "Cancelada - Ordem cancelada",
		"AUTHORIZATION_DENIED":  "Autorização Negada - Orçamento não aprovado",
	}

	newStatusMsg := statusMessages[newStatus]
	if newStatusMsg == "" {
		newStatusMsg = newStatus
	}

	return fmt.Sprintf(`Olá %s,

A sua Ordem de Serviço #%s teve o status atualizado.

Status Anterior: %s
Novo Status: %s

%s

Para mais informações, entre em contato conosco.

Atenciosamente,
Equipe Oficina Tech
`, customerName, serviceOrderID, oldStatus, newStatus, newStatusMsg)
}

// sendEmail envia o email via SMTP
func (s *SMTPEmailService) sendEmail(to, subject, body string) error {
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s\r\n", s.from, to, subject, body))

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	err := smtp.SendMail(addr, auth, s.from, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("falha ao enviar email: %w", err)
	}

	return nil
}

// NoOpEmailService descarta todos os envios silenciosamente.
type NoOpEmailService struct{}

func NewNoOpEmailService() *NoOpEmailService { return &NoOpEmailService{} }

func (n *NoOpEmailService) SendStatusUpdateEmail(_, _, _, _, _ string) error { return nil }

// getEnv obtém variável de ambiente com valor padrão
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return strings.TrimSpace(value)
}
