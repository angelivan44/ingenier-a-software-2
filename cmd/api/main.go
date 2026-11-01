// Punto de entrada del servidor del acortador de enlaces.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/angelivan44/ingenier-a-software-2/internal/links"
	"github.com/angelivan44/ingenier-a-software-2/internal/server"
)

const connectTimeout = 10 * time.Second

func main() {
	// Vercel define PORT; en local se usa 3000.
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	store, err := newStore()
	if err != nil {
		log.Fatalf("no se pudo iniciar el almacenamiento: %v", err)
	}
	service := links.NewService(store)

	log.Printf("servidor escuchando en el puerto %s", port)
	log.Fatal(http.ListenAndServe(":"+port, server.New(service)))
}

// newStore elige el almacenamiento según el entorno: Postgres si hay
// DATABASE_URL y, si no, memoria para probar sin base de datos.
func newStore() (links.Store, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Print("DATABASE_URL no definida: los enlaces se guardan en memoria")
		return links.NewMemoryStore(), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return links.NewPostgresStore(ctx, databaseURL)
}
