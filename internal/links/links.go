// Package links contiene las reglas de negocio de los enlaces cortos.
package links

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

const (
	codeLength  = 7
	maxAttempts = 5
	alphabet    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	// ErrInvalidURL indica que la URL recibida no se puede acortar.
	ErrInvalidURL = errors.New("la URL debe ser http o https e incluir un dominio")
	// ErrCodeExists indica que el código generado ya está en uso.
	ErrCodeExists = errors.New("el código ya existe")
	// ErrNotFound indica que no hay un enlace con ese código.
	ErrNotFound = errors.New("enlace no encontrado")
)

// Link es un enlace corto y la URL a la que apunta.
type Link struct {
	Code      string
	URL       string
	CreatedAt time.Time
}

// Store guarda los enlaces. Al ser una interfaz, el almacenamiento
// se puede cambiar sin tocar las reglas de negocio.
type Store interface {
	Save(ctx context.Context, link Link) error
	Find(ctx context.Context, code string) (Link, error)
}

// Service aplica las reglas para crear enlaces.
type Service struct {
	store Store
}

// NewService crea un Service que usa el almacenamiento indicado.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create valida la URL y guarda un enlace con un código nuevo.
func (s *Service) Create(ctx context.Context, rawURL string) (Link, error) {
	target, err := validateURL(rawURL)
	if err != nil {
		return Link{}, err
	}

	// Una colisión es muy improbable, pero se reintenta con otro código.
	for range maxAttempts {
		code, err := generateCode()
		if err != nil {
			return Link{}, fmt.Errorf("generar código: %w", err)
		}

		link := Link{Code: code, URL: target, CreatedAt: time.Now().UTC()}
		err = s.store.Save(ctx, link)
		if err == nil {
			return link, nil
		}
		if !errors.Is(err, ErrCodeExists) {
			return Link{}, fmt.Errorf("guardar enlace: %w", err)
		}
	}
	return Link{}, fmt.Errorf("sin código libre tras %d intentos: %w", maxAttempts, ErrCodeExists)
}

// Resolve busca el enlace asociado a un código.
func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	link, err := s.store.Find(ctx, code)
	if err != nil {
		return Link{}, fmt.Errorf("buscar enlace: %w", err)
	}
	return link, nil
}

// validateURL acepta solo URLs absolutas http o https con dominio.
func validateURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return "", ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidURL
	}
	if parsed.Hostname() == "" {
		return "", ErrInvalidURL
	}
	return trimmed, nil
}

// generateCode crea un código aleatorio de letras y números.
func generateCode() (string, error) {
	limit := big.NewInt(int64(len(alphabet)))
	code := make([]byte, codeLength)
	for i := range code {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		code[i] = alphabet[n.Int64()]
	}
	return string(code), nil
}
