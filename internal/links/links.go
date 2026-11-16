// Package links contiene las reglas de negocio de los enlaces cortos.
package links

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
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
	// ErrInvalidAlias indica que el alias pedido no cumple el formato.
	ErrInvalidAlias = errors.New("el alias debe tener de 3 a 30 letras, números, guiones o guiones bajos y no ser una ruta reservada")
	// ErrAliasTaken indica que otro enlace ya usa el alias pedido.
	ErrAliasTaken = errors.New("el alias ya está en uso")
)

// aliasPattern define los caracteres y el largo permitidos para un alias.
var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,30}$`)

// reservedAliases son rutas del servidor: un alias con ese nombre nunca
// llegaría a redirigir.
var reservedAliases = map[string]bool{"api": true, "health": true}

// NewLink son los datos para crear un enlace. Alias es opcional: si está
// vacío se genera un código aleatorio.
type NewLink struct {
	URL   string
	Alias string
}

// Link es un enlace corto y la URL a la que apunta.
type Link struct {
	Code      string
	URL       string
	Clicks    int64
	CreatedAt time.Time
}

// Store guarda los enlaces. Al ser una interfaz, el almacenamiento
// se puede cambiar sin tocar las reglas de negocio.
type Store interface {
	Save(ctx context.Context, link Link) error
	Find(ctx context.Context, code string) (Link, error)
	// IncrementClicks suma un clic y devuelve el enlace actualizado.
	// Es una sola operación para no perder clics simultáneos.
	IncrementClicks(ctx context.Context, code string) (Link, error)
}

// Service aplica las reglas para crear enlaces.
type Service struct {
	store Store
}

// NewService crea un Service que usa el almacenamiento indicado.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create valida los datos y guarda un enlace con el alias pedido o con un
// código nuevo.
func (s *Service) Create(ctx context.Context, input NewLink) (Link, error) {
	target, err := validateURL(input.URL)
	if err != nil {
		return Link{}, err
	}

	if input.Alias != "" {
		return s.createWithAlias(ctx, target, input.Alias)
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

// createWithAlias guarda el enlace con el código que eligió el usuario.
// No se reintenta: si el alias está ocupado, el usuario debe elegir otro.
func (s *Service) createWithAlias(ctx context.Context, target, alias string) (Link, error) {
	if err := validateAlias(alias); err != nil {
		return Link{}, err
	}

	link := Link{Code: alias, URL: target, CreatedAt: time.Now().UTC()}
	err := s.store.Save(ctx, link)
	if errors.Is(err, ErrCodeExists) {
		return Link{}, ErrAliasTaken
	}
	if err != nil {
		return Link{}, fmt.Errorf("guardar enlace: %w", err)
	}
	return link, nil
}

// Resolve busca el enlace asociado a un código sin contar un clic.
func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	link, err := s.store.Find(ctx, code)
	if err != nil {
		return Link{}, fmt.Errorf("buscar enlace: %w", err)
	}
	return link, nil
}

// Visit registra un clic y devuelve el enlace visitado.
func (s *Service) Visit(ctx context.Context, code string) (Link, error) {
	link, err := s.store.IncrementClicks(ctx, code)
	if err != nil {
		return Link{}, fmt.Errorf("registrar clic: %w", err)
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

// validateAlias acepta solo alias con el formato permitido y que no
// coincidan con una ruta del servidor.
func validateAlias(alias string) error {
	if !aliasPattern.MatchString(alias) || reservedAliases[strings.ToLower(alias)] {
		return ErrInvalidAlias
	}
	return nil
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
