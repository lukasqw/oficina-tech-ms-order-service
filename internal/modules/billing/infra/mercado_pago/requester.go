package mercado_pago

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	mprequester "github.com/mercadopago/sdk-go/pkg/requester"
)

// RewritingRequester implementa requester.Requester do SDK e redireciona todas
// as chamadas HTTP para um host alternativo (MP_BASE_URL).
// Usado exclusivamente em ambiente BDD/E2E para apontar para o mockmp local.
type RewritingRequester struct {
	inner  mprequester.Requester
	scheme string
	host   string
}

// NewRewritingRequester cria um Requester que substitui scheme+host de todas as
// requisições pelo baseURL informado (ex: "http://localhost:9999").
// Injete via config.WithHTTPClient(NewRewritingRequester(baseURL)).
func NewRewritingRequester(baseURL string) mprequester.Requester {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Host == "" {
		// Fallback: usa o http.Client padrão sem reescrita
		return &http.Client{Timeout: 10 * time.Second}
	}
	return &RewritingRequester{
		inner:  &http.Client{Timeout: 10 * time.Second},
		scheme: u.Scheme,
		host:   u.Host,
	}
}

func (r *RewritingRequester) Do(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = r.scheme
	cloned.URL.Host = r.host
	cloned.Host = r.host
	return r.inner.Do(cloned)
}
