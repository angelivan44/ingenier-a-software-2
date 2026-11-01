package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angelivan44/ingenier-a-software-2/internal/links"
)

// failingStore simula una falla del almacenamiento.
type failingStore struct{}

func (failingStore) Save(context.Context, links.Link) error {
	return errors.New("base de datos caída")
}

func (failingStore) Find(context.Context, string) (links.Link, error) {
	return links.Link{}, errors.New("base de datos caída")
}

func (failingStore) IncrementClicks(context.Context, string) (links.Link, error) {
	return links.Link{}, errors.New("base de datos caída")
}

func newTestHandler() http.Handler {
	return New(links.NewService(links.NewMemoryStore()))
}

func postLink(handler http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	newTestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("cuerpo inesperado: %s", body)
	}
}

func TestCreateLinkRespondeConEnlaceCorto(t *testing.T) {
	rec := postLink(newTestHandler(), `{"url":"https://example.com/articulo"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, se esperaba %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if got.URL != "https://example.com/articulo" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.ShortURL != "http://example.com/"+got.Code {
		t.Errorf("ShortURL = %q, código = %q", got.ShortURL, got.Code)
	}
}

func TestCreateLinkRechazaPeticionesInvalidas(t *testing.T) {
	casos := map[string]string{
		"JSON roto":         `{"url":`,
		"sin campo url":     `{}`,
		"URL inválida":      `{"url":"no es una url"}`,
		"esquema peligroso": `{"url":"javascript:alert(1)"}`,
		"cuerpo muy grande": `{"url":"https://example.com/` + strings.Repeat("a", maxBodyBytes) + `"}`,
	}

	for nombre, body := range casos {
		t.Run(nombre, func(t *testing.T) {
			rec := postLink(newTestHandler(), body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// createLink crea un enlace de prueba y devuelve su respuesta.
func createLink(t *testing.T, handler http.Handler) linkResponse {
	t.Helper()

	rec := postLink(handler, `{"url":"https://example.com"}`)
	var link linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &link); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	return link
}

// statsOf consulta los datos de un enlace.
func statsOf(t *testing.T, handler http.Handler, code string) linkResponse {
	t.Helper()

	rec := get(handler, "/api/links/"+code)
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusOK)
	}
	var stats linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	return stats
}

func TestStatsCuentaLasVisitas(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	get(handler, "/"+link.Code)
	get(handler, "/"+link.Code)

	stats := statsOf(t, handler, link.Code)
	if stats.Clicks != 2 {
		t.Errorf("clics = %d, se esperaban 2", stats.Clicks)
	}
	if stats.URL != "https://example.com" || stats.CreatedAt.IsZero() {
		t.Errorf("datos incompletos: %+v", stats)
	}
}

func TestStatsNoCuentaComoClic(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	statsOf(t, handler, link.Code)

	if stats := statsOf(t, handler, link.Code); stats.Clicks != 0 {
		t.Errorf("clics = %d, se esperaban 0", stats.Clicks)
	}
}

func TestStatsRespondeNotFoundSiNoExiste(t *testing.T) {
	if rec := get(newTestHandler(), "/api/links/noexiste"); rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusNotFound)
	}
}

func TestStatsNoExponeErroresInternos(t *testing.T) {
	rec := get(New(links.NewService(failingStore{})), "/api/links/abc1234")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "base de datos") {
		t.Fatalf("la respuesta expone el error interno: %s", rec.Body)
	}
}

func TestRedirectHeadNoCuentaComoClic(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	req := httptest.NewRequest(http.MethodHead, "/"+link.Code, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusFound)
	}
	if stats := statsOf(t, handler, link.Code); stats.Clicks != 0 {
		t.Errorf("clics = %d, se esperaban 0", stats.Clicks)
	}
}

func TestRedirectEnviaALaURLOriginal(t *testing.T) {
	handler := newTestHandler()
	created := postLink(handler, `{"url":"https://example.com/articulo?id=1"}`)

	var link linkResponse
	if err := json.Unmarshal(created.Body.Bytes(), &link); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}

	rec := get(handler, "/"+link.Code)

	if rec.Code != http.StatusFound {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "https://example.com/articulo?id=1" {
		t.Errorf("Location = %q", got)
	}
}

func TestRedirectRespondeNotFoundSiNoExiste(t *testing.T) {
	rec := get(newTestHandler(), "/noexiste")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusNotFound)
	}
}

func TestRedirectNoExponeErroresInternos(t *testing.T) {
	rec := get(New(links.NewService(failingStore{})), "/abc1234")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "base de datos") {
		t.Fatalf("la respuesta expone el error interno: %s", rec.Body)
	}
}

func TestHealthNoChocaConLaRutaDeCodigos(t *testing.T) {
	if rec := get(newTestHandler(), "/health"); rec.Code != http.StatusOK {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusOK)
	}
}

func TestCreateLinkNoExponeErroresInternos(t *testing.T) {
	handler := New(links.NewService(failingStore{}))

	rec := postLink(handler, `{"url":"https://example.com"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "base de datos") {
		t.Fatalf("la respuesta expone el error interno: %s", rec.Body)
	}
}
