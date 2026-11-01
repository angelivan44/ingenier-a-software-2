// Punto de entrada del servidor del acortador de enlaces.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/angelhuayas/acortador-enlaces/internal/server"
)

func main() {
	// Vercel define PORT; en local se usa 3000.
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("servidor escuchando en el puerto %s", port)
	log.Fatal(http.ListenAndServe(":"+port, server.New()))
}
