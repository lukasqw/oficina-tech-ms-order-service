package handlers

import (
	"fmt"
	"net/http"
)

// ResultHandler serve a página HTML de destino após o redirect do Mercado Pago.
// Rotas: GET /payments/result?status=success|pending|failure&order=<id>
// Não requer autenticação — é o endpoint de redirect público configurado no Order MP.
type ResultHandler struct{}

func NewResultHandler() *ResultHandler {
	return &ResultHandler{}
}

func (h *ResultHandler) Handle(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	orderID := r.URL.Query().Get("order")

	switch status {
	case "success", "pending", "failure":
	default:
		http.Error(w, `{"errors":[{"code":"VALIDATION_FAILED","message":"status inválido: use success, pending ou failure"}]}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	switch status {
	case "success":
		_, _ = fmt.Fprintf(w, resultHTML, "Pagamento Aprovado", "#22c55e",
			"Seu pagamento foi aprovado com sucesso.",
			"A ordem de serviço <strong>"+orderID+"</strong> está sendo processada.",
			"Você receberá um e-mail de confirmação em breve.")
	case "pending":
		_, _ = fmt.Fprintf(w, resultHTML, "Pagamento Pendente", "#f59e0b",
			"Seu pagamento está sendo processado.",
			"A ordem de serviço <strong>"+orderID+"</strong> aguarda confirmação.",
			"Você será notificado por e-mail quando o pagamento for confirmado.")
	case "failure":
		_, _ = fmt.Fprintf(w, resultHTML, "Pagamento não Concluído", "#ef4444",
			"Não foi possível processar o pagamento.",
			"A ordem de serviço <strong>"+orderID+"</strong> continua disponível.",
			"Você pode tentar novamente através do aplicativo.")
	}
}

const resultHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — Oficina Tech</title>
  <style>
    body { font-family: system-ui, sans-serif; display:flex; justify-content:center;
           align-items:center; min-height:100vh; margin:0; background:#f8fafc; }
    .card { background:#fff; border-radius:12px; padding:40px; max-width:440px;
            width:90%%; text-align:center; box-shadow:0 2px 16px rgba(0,0,0,.08); }
    .icon { font-size:48px; margin-bottom:16px; }
    h1 { color:%s; margin:0 0 12px; font-size:1.5rem; }
    p  { color:#475569; margin:8px 0; line-height:1.5; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">&#128274;</div>
    <h1>%s</h1>
    <p>%s</p>
    <p>%s</p>
  </div>
</body>
</html>`
