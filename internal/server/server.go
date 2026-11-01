// Package server define las rutas HTTP de la aplicación.
package server

import (
	"encoding/json"
	"log"
	"net/http"
)

// New devuelve el manejador HTTP con todas las rutas registradas.
func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	return mux
}

// health permite comprobar que el servicio está activo.
func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("no se pudo escribir la respuesta: %v", err)
	}
}
