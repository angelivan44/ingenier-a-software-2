// Punto de entrada del servidor del acortador de enlaces.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/angelivan44/ingenier-a-software-2/internal/links"
	"github.com/angelivan44/ingenier-a-software-2/internal/server"
)

func main() {
	// Vercel define PORT; en local se usa 3000.
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	service := links.NewService(links.NewMemoryStore())

	log.Printf("servidor escuchando en el puerto %s", port)
	log.Fatal(http.ListenAndServe(":"+port, server.New(service)))
}
