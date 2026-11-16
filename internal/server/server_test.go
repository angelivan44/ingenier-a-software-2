package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func (failingStore) Delete(context.Context, string) error {
	return errors.New("base de datos caída")
}

// testHost es el dominio del acortador en las pruebas. No es example.com,
// el host por defecto de httptest, porque las URLs de destino usan ese dominio.
const testHost = "corto.test"

func newTestHandler() http.Handler {
	return New(links.NewService(links.NewMemoryStore()))
}

func postLink(handler http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	req.Host = testHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHomeMuestraElFormulario(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = testHost
	rec := httptest.NewRecorder()

	newTestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `id="formulario"`) {
		t.Error("la página no incluye el formulario")
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = testHost
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
	if got.ShortURL != "http://corto.test/"+got.Code {
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
	req.Host = testHost
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
	req.Host = testHost
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

func TestCreateLinkConAlias(t *testing.T) {
	handler := newTestHandler()

	rec := postLink(handler, `{"url":"https://example.com/articulo","alias":"articulo"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, se esperaba %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	var got linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if got.ShortURL != "http://corto.test/articulo" {
		t.Errorf("ShortURL = %q", got.ShortURL)
	}

	redirect := get(handler, "/articulo")
	if redirect.Code != http.StatusFound || redirect.Header().Get("Location") != "https://example.com/articulo" {
		t.Errorf("redirección = %d %q", redirect.Code, redirect.Header().Get("Location"))
	}
}

func TestCreateLinkRechazaAliasInvalido(t *testing.T) {
	rec := postLink(newTestHandler(), `{"url":"https://example.com","alias":"a b"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateLinkRespondeConflictoSiElAliasExiste(t *testing.T) {
	handler := newTestHandler()
	postLink(handler, `{"url":"https://example.com","alias":"promo"}`)

	rec := postLink(handler, `{"url":"https://otro.com","alias":"promo"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusConflict)
	}

	// El enlace original no debe cambiar.
	if stats := statsOf(t, handler, "promo"); stats.URL != "https://example.com" {
		t.Errorf("URL = %q, el alias se sobrescribió", stats.URL)
	}
}

func deleteLink(handler http.Handler, code, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/links/"+code, nil)
	req.Host = testHost
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCreateLinkEntregaTokenSoloAlCrear(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	if link.DeleteToken == "" {
		t.Fatal("la respuesta de creación no incluye delete_token")
	}
	if rec := get(handler, "/api/links/"+link.Code); strings.Contains(rec.Body.String(), "delete_token") {
		t.Errorf("la consulta expone el token: %s", rec.Body)
	}
}

func TestDeleteLinkConTokenCorrecto(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	rec := deleteLink(handler, link.Code, "Bearer "+link.DeleteToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, se esperaba %d: %s", rec.Code, http.StatusNoContent, rec.Body)
	}
	if rec := get(handler, "/"+link.Code); rec.Code != http.StatusNotFound {
		t.Errorf("redirección = %d, se esperaba %d tras eliminar", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteLinkRespuestasDeError(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	casos := []struct {
		nombre        string
		code          string
		authorization string
		esperado      int
	}{
		{"sin cabecera", link.Code, "", http.StatusUnauthorized},
		{"sin Bearer", link.Code, link.DeleteToken, http.StatusUnauthorized},
		{"Bearer vacío", link.Code, "Bearer   ", http.StatusUnauthorized},
		{"token incorrecto", link.Code, "Bearer otro", http.StatusForbidden},
		{"código inexistente", "noexiste", "Bearer " + link.DeleteToken, http.StatusNotFound},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if rec := deleteLink(handler, c.code, c.authorization); rec.Code != c.esperado {
				t.Fatalf("código = %d, se esperaba %d", rec.Code, c.esperado)
			}
		})
	}
}

func TestDeleteLinkNoExponeErroresInternos(t *testing.T) {
	rec := deleteLink(New(links.NewService(failingStore{})), "abc1234", "Bearer token")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("código = %d, se esperaba %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "base de datos") {
		t.Fatalf("la respuesta expone el error interno: %s", rec.Body)
	}
}

func TestCreateLinkConExpiracion(t *testing.T) {
	rec := postLink(newTestHandler(), `{"url":"https://example.com","expires_in_days":7}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, se esperaba %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(got.CreatedAt.AddDate(0, 0, 7)) {
		t.Errorf("expires_at = %v, created_at = %v", got.ExpiresAt, got.CreatedAt)
	}
}

func TestCreateLinkSinExpiracionOmiteElCampo(t *testing.T) {
	rec := postLink(newTestHandler(), `{"url":"https://example.com"}`)

	if strings.Contains(rec.Body.String(), "expires_at") {
		t.Errorf("la respuesta incluye expires_at: %s", rec.Body)
	}
}

func TestCreateLinkRechazaExpiracionFueraDeRango(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://example.com","expires_in_days":-1}`,
		`{"url":"https://example.com","expires_in_days":366}`,
		`{"url":"https://example.com","expires_in_days":"7"}`,
	} {
		if rec := postLink(newTestHandler(), body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: código = %d, se esperaba %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestRedirectRespondeGoneSiElEnlaceVencio(t *testing.T) {
	ahora := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	handler := New(links.NewServiceWithClock(links.NewMemoryStore(), func() time.Time { return ahora }))
	postLink(handler, `{"url":"https://example.com","alias":"oferta","expires_in_days":1}`)

	ahora = ahora.AddDate(0, 0, 1)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/oferta", nil)
		req.Host = testHost
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusGone {
			t.Errorf("%s: código = %d, se esperaba %d", method, rec.Code, http.StatusGone)
		}
	}

	// La consulta sigue disponible para ver los clics.
	if stats := statsOf(t, handler, "oferta"); stats.ExpiresAt == nil {
		t.Error("la consulta no muestra expires_at")
	}
}

// Defecto: un enlace hacia el propio acortador crea redirecciones en cadena,
// y dos enlaces que se apuntan entre sí forman un bucle.
func TestCreateLinkRechazaURLDelPropioAcortador(t *testing.T) {
	handler := newTestHandler()
	link := createLink(t, handler)

	for _, url := range []string{
		link.ShortURL,
		"https://corto.test/" + link.Code,
		"http://CORTO.test/otro",
		"http://corto.test:80/otro",
	} {
		rec := postLink(handler, `{"url":"`+url+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: código = %d, se esperaba %d", url, rec.Code, http.StatusBadRequest)
		}
	}

	// Un subdominio u otro sitio sí se permiten.
	if rec := postLink(handler, `{"url":"https://blog.corto.test/"}`); rec.Code != http.StatusCreated {
		t.Errorf("subdominio: código = %d, se esperaba %d", rec.Code, http.StatusCreated)
	}
}

// Defecto: el decodificador lee solo el primer valor JSON y descarta el resto
// del cuerpo, así una petición mal formada se acepta en silencio.
func TestCreateLinkRechazaDatosDespuesDelJSON(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://example.com"} basura`,
		`{"url":"https://example.com"}{"url":"https://otro.com"}`,
	} {
		if rec := postLink(newTestHandler(), body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: código = %d, se esperaba %d", body, rec.Code, http.StatusBadRequest)
		}
	}

	// Un salto de línea al final es habitual y se acepta.
	if rec := postLink(newTestHandler(), "{\"url\":\"https://example.com\"}\n"); rec.Code != http.StatusCreated {
		t.Errorf("con salto de línea: código = %d, se esperaba %d", rec.Code, http.StatusCreated)
	}
}
