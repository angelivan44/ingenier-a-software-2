// Package server define las rutas HTTP de la aplicación.
package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/angelivan44/ingenier-a-software-2/internal/links"
)

// maxBodyBytes limita el tamaño del cuerpo para rechazar peticiones abusivas.
const maxBodyBytes = 4 << 10

// Server agrupa las dependencias de los manejadores HTTP.
type Server struct {
	links *links.Service
}

// New devuelve el manejador HTTP con todas las rutas registradas.
func New(service *links.Service) http.Handler {
	s := &Server{links: service}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links/{code}", s.linkStats)
	mux.HandleFunc("GET /{code}", s.redirect)
	return mux
}

// health permite comprobar que el servicio está activo.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createLinkRequest struct {
	URL string `json:"url"`
}

type linkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ShortURL  string    `json:"short_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

func newLinkResponse(r *http.Request, link links.Link) linkResponse {
	return linkResponse{
		Code:      link.Code,
		URL:       link.URL,
		ShortURL:  baseURL(r) + "/" + link.Code,
		Clicks:    link.Clicks,
		CreatedAt: link.CreatedAt,
	}
}

// createLink recibe una URL y responde con su enlace corto.
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "el cuerpo debe ser JSON con el campo url")
		return
	}

	link, err := s.links.Create(r.Context(), req.URL)
	if errors.Is(err, links.ErrInvalidURL) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		log.Printf("no se pudo crear el enlace: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo crear el enlace")
		return
	}

	writeJSON(w, http.StatusCreated, newLinkResponse(r, link))
}

// linkStats muestra los datos de un enlace sin contarlo como clic.
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
		lookup = s.links.Resolve
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
