// Package links contiene las reglas de negocio de los enlaces cortos.
package links

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	tokenLength = 32
	maxAttempts = 5
	// maxExpirationDays es el plazo máximo que se puede pedir para que un
	// enlace venza.
	maxExpirationDays = 365
	alphabet          = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
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
	// ErrForbidden indica que el token no permite eliminar el enlace.
	ErrForbidden = errors.New("el token no permite eliminar este enlace")
	// ErrInvalidExpiration indica que el plazo de expiración está fuera de rango.
	ErrInvalidExpiration = errors.New("expires_in_days debe estar entre 1 y 365")
	// ErrExpired indica que el enlace ya venció.
	ErrExpired = errors.New("el enlace expiró")
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
	// ExpiresInDays es opcional: 0 significa que el enlace no vence.
	ExpiresInDays int
}

// Link es un enlace corto y la URL a la que apunta.
type Link struct {
	Code      string
	URL       string
	Clicks    int64
	CreatedAt time.Time
	// ExpiresAt es el momento en que el enlace deja de redirigir. Si es
	// cero, el enlace no vence.
	ExpiresAt time.Time
	// DeleteTokenHash es el hash del token para eliminar el enlace. Se guarda
	// el hash y no el token, así una filtración de la base no permite borrar.
	DeleteTokenHash string
	// DeleteToken solo se llena al crear el enlace, para entregarlo una vez.
	// No se guarda.
	DeleteToken string
}

// Store guarda los enlaces. Al ser una interfaz, el almacenamiento
// se puede cambiar sin tocar las reglas de negocio.
type Store interface {
	Save(ctx context.Context, link Link) error
	Find(ctx context.Context, code string) (Link, error)
	// IncrementClicks suma un clic y devuelve el enlace actualizado.
	// Es una sola operación para no perder clics simultáneos.
	IncrementClicks(ctx context.Context, code string) (Link, error)
	// Delete borra el enlace o devuelve ErrNotFound si no existe.
	Delete(ctx context.Context, code string) error
}

// Expired indica si el enlace ya venció en el momento indicado. Vence
// justo en ExpiresAt, no un instante después.
func (l Link) Expired(now time.Time) bool {
	return !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt)
}

// Service aplica las reglas para crear enlaces.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService crea un Service que usa el almacenamiento indicado.
func NewService(store Store) *Service {
	return NewServiceWithClock(store, time.Now)
}

// NewServiceWithClock permite fijar la hora actual, así las pruebas pueden
// comprobar la expiración sin esperar días.
func NewServiceWithClock(store Store, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

// Create valida los datos y guarda un enlace con el alias pedido o con un
// código nuevo.
func (s *Service) Create(ctx context.Context, input NewLink) (Link, error) {
	target, err := validateURL(input.URL)
	if err != nil {
		return Link{}, err
	}
	if input.ExpiresInDays < 0 || input.ExpiresInDays > maxExpirationDays {
		return Link{}, ErrInvalidExpiration
	}

	if input.Alias != "" {
		return s.createWithAlias(ctx, target, input)
	}

	// Una colisión es muy improbable, pero se reintenta con otro código.
	for range maxAttempts {
		code, err := generateCode()
		if err != nil {
			return Link{}, fmt.Errorf("generar código: %w", err)
		}

		link, err := s.newLink(code, target, input.ExpiresInDays)
		if err != nil {
			return Link{}, err
		}
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
func (s *Service) createWithAlias(ctx context.Context, target string, input NewLink) (Link, error) {
	if err := validateAlias(input.Alias); err != nil {
		return Link{}, err
	}

	link, err := s.newLink(input.Alias, target, input.ExpiresInDays)
	if err != nil {
		return Link{}, err
	}
	err = s.store.Save(ctx, link)
	if errors.Is(err, ErrCodeExists) {
		return Link{}, ErrAliasTaken
	}
	if err != nil {
		return Link{}, fmt.Errorf("guardar enlace: %w", err)
	}
	return link, nil
}

// newLink arma un enlace nuevo con su token para eliminarlo y, si se pidió,
// su fecha de expiración.
func (s *Service) newLink(code, target string, expiresInDays int) (Link, error) {
	token, err := randomString(tokenLength)
	if err != nil {
		return Link{}, fmt.Errorf("generar token: %w", err)
	}

	// Postgres guarda microsegundos: se trunca para que el enlace devuelto
	// coincida con el guardado.
	now := s.now().UTC().Truncate(time.Microsecond)
	link := Link{
		Code:            code,
		URL:             target,
		CreatedAt:       now,
		DeleteTokenHash: hashToken(token),
		DeleteToken:     token,
	}
	if expiresInDays > 0 {
		link.ExpiresAt = now.AddDate(0, 0, expiresInDays)
	}
	return link, nil
}

// Delete elimina el enlace si el token coincide con el que se entregó al
// crearlo.
func (s *Service) Delete(ctx context.Context, code, token string) error {
	link, err := s.store.Find(ctx, code)
	if err != nil {
		return fmt.Errorf("buscar enlace: %w", err)
	}

	// Los enlaces creados antes de esta función no tienen token: nadie
	// puede eliminarlos. La comparación en tiempo constante evita deducir
	// el hash midiendo cuánto tarda la respuesta.
	if token == "" || link.DeleteTokenHash == "" ||
		subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(link.DeleteTokenHash)) != 1 {
		return ErrForbidden
	}

	if err := s.store.Delete(ctx, code); err != nil {
		return fmt.Errorf("eliminar enlace: %w", err)
	}
	return nil
}

// hashToken devuelve el SHA-256 del token en hexadecimal.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Resolve busca el enlace asociado a un código sin contar un clic.
func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	link, err := s.store.Find(ctx, code)
	if err != nil {
		return Link{}, fmt.Errorf("buscar enlace: %w", err)
	}
	return link, nil
}

// Check busca el enlace para redirigir sin contar un clic. Devuelve
// ErrExpired si ya venció.
func (s *Service) Check(ctx context.Context, code string) (Link, error) {
	link, err := s.Resolve(ctx, code)
	if err != nil {
		return Link{}, err
	}
	if link.Expired(s.now()) {
		return Link{}, ErrExpired
	}
	return link, nil
}

// Visit registra un clic y devuelve el enlace visitado. Un enlace vencido
// devuelve ErrExpired y no suma el clic.
func (s *Service) Visit(ctx context.Context, code string) (Link, error) {
	if _, err := s.Check(ctx, code); err != nil {
		return Link{}, err
	}

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
	return randomString(codeLength)
}

// randomString crea una cadena aleatoria de letras y números.
func randomString(length int) (string, error) {
	limit := big.NewInt(int64(len(alphabet)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}
