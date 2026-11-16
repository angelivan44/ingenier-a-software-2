// Package server define las rutas HTTP de la aplicación.
package server

import (
	_ "embed" // Permite incluir index.html dentro del binario.
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/angelivan44/ingenier-a-software-2/internal/links"
)

// maxBodyBytes limita el tamaño del cuerpo para rechazar peticiones abusivas.
const maxBodyBytes = 4 << 10

//go:embed index.html
var indexPage []byte

// Server agrupa las dependencias de los manejadores HTTP.
type Server struct {
	links *links.Service
}

// New devuelve el manejador HTTP con todas las rutas registradas.
func New(service *links.Service) http.Handler {
	s := &Server{links: service}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links/{code}", s.linkStats)
	mux.HandleFunc("DELETE /api/links/{code}", s.deleteLink)
	mux.HandleFunc("GET /{code}", s.redirect)
	return mux
}

// home muestra la página para crear enlaces desde el navegador.
func (s *Server) home(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(indexPage); err != nil {
		log.Printf("no se pudo escribir la página: %v", err)
	}
}

// health permite comprobar que el servicio está activo.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createLinkRequest struct {
	URL           string `json:"url"`
	Alias         string `json:"alias"`
	ExpiresInDays int    `json:"expires_in_days"`
}

type linkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ShortURL  string    `json:"short_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt se omite si el enlace no vence.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// DeleteToken solo se envía al crear el enlace.
	DeleteToken string `json:"delete_token,omitempty"`
}

func newLinkResponse(r *http.Request, link links.Link) linkResponse {
	response := linkResponse{
		Code:      link.Code,
		URL:       link.URL,
		ShortURL:  baseURL(r) + "/" + link.Code,
		Clicks:    link.Clicks,
		CreatedAt: link.CreatedAt,
	}
	if !link.ExpiresAt.IsZero() {
		response.ExpiresAt = &link.ExpiresAt
	}
	return response
}

// createLink recibe una URL, y opcionalmente un alias, y responde con su
// enlace corto.
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "el cuerpo debe ser un solo objeto JSON con el campo url")
		return
	}

	link, err := s.links.Create(r.Context(), links.NewLink{
		URL:           req.URL,
		Alias:         req.Alias,
		ExpiresInDays: req.ExpiresInDays,
		ServiceHost:   r.Host,
	})
	if errors.Is(err, links.ErrInvalidURL) || errors.Is(err, links.ErrInvalidAlias) ||
		errors.Is(err, links.ErrInvalidExpiration) || errors.Is(err, links.ErrSelfLink) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, links.ErrAliasTaken) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		log.Printf("no se pudo crear el enlace: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo crear el enlace")
		return
	}

	response := newLinkResponse(r, link)
	response.DeleteToken = link.DeleteToken
	writeJSON(w, http.StatusCreated, response)
}

// deleteLink elimina un enlace si la petición trae el token que se entregó
// al crearlo, en la cabecera Authorization: Bearer <token>.
func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		writeError(w, http.StatusUnauthorized, "falta el token para eliminar el enlace")
		return
	}

	err := s.links.Delete(r.Context(), r.PathValue("code"), strings.TrimSpace(token))
	if errors.Is(err, links.ErrForbidden) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		writeLookupError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// linkStats muestra los datos de un enlace sin contarlo como clic. También
// responde para enlaces vencidos, así el dueño puede ver sus clics.
func (s *Server) linkStats(w http.ResponseWriter, r *http.Request) {
	link, err := s.links.Resolve(r.Context(), r.PathValue("code"))
	if err != nil {
		writeLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newLinkResponse(r, link))
}

// redirect envía al visitante a la URL original y cuenta el clic.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	// Una petición HEAD solo comprueba el enlace: nadie lo visitó de verdad.
	lookup := s.links.Visit
	if r.Method == http.MethodHead {
		lookup = s.links.Check
	}

	link, err := lookup(r.Context(), r.PathValue("code"))
	if err != nil {
		writeLookupError(w, err)
		return
	}

	// 302 y no 301: el navegador no guarda la redirección en caché,
	// así cada visita pasa por el servidor y se cuenta como clic.
	http.Redirect(w, r, link.URL, http.StatusFound)
}

// writeLookupError responde a un error al buscar un enlace.
func writeLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, links.ErrNotFound) {
		writeError(w, http.StatusNotFound, "enlace no encontrado")
		return
	}
	if errors.Is(err, links.ErrExpired) {
		writeError(w, http.StatusGone, err.Error())
		return
	}
	log.Printf("no se pudo obtener el enlace: %v", err)
	writeError(w, http.StatusInternalServerError, "no se pudo obtener el enlace")
}

// baseURL arma la dirección pública a partir de la petición recibida.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// decodeJSON lee un único valor JSON. Decode solo lee el primero, así que
// después se exige el fin del cuerpo: datos extra indican una petición mal
// formada. Los espacios y saltos de línea al final se permiten.
func decodeJSON(body io.Reader, dst any) error {
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("hay datos después del JSON")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("no se pudo escribir la respuesta: %v", err)
	}
}
