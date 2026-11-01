package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angelhuayas/acortador-enlaces/internal/links"
)

// failingStore simula una falla del almacenamiento.
type failingStore struct{}

func (failingStore) Save(context.Context, links.Link) error {
	return errors.New("base de datos caída")
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
