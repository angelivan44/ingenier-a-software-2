package links

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

// fakeStore simula colisiones: falla las primeras veces que se guarda.
type fakeStore struct {
	failures int
	err      error
	calls    int
}

func (f *fakeStore) Save(context.Context, Link) error {
	f.calls++
	if f.calls <= f.failures {
		return f.err
	}
	return nil
}

func TestCreateGuardaEnlaceConCodigoValido(t *testing.T) {
	service := NewService(NewMemoryStore())

	link, err := service.Create(context.Background(), "  https://example.com/ruta?a=1  ")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if link.URL != "https://example.com/ruta?a=1" {
		t.Errorf("URL = %q, se esperaba sin espacios", link.URL)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9]{7}$`).MatchString(link.Code) {
		t.Errorf("código con formato inválido: %q", link.Code)
	}
	if link.CreatedAt.IsZero() {
		t.Error("CreatedAt no debería estar vacío")
	}
}

func TestCreateRechazaURLInvalida(t *testing.T) {
	casos := map[string]string{
		"vacía":        "",
		"sin esquema":  "example.com",
		"esquema ftp":  "ftp://example.com",
		"javascript":   "javascript:alert(1)",
		"sin dominio":  "https://",
		"solo puerto":  "http://:8080",
		"solo espacio": "   ",
	}

	for nombre, url := range casos {
		t.Run(nombre, func(t *testing.T) {
			service := NewService(NewMemoryStore())
			if _, err := service.Create(context.Background(), url); !errors.Is(err, ErrInvalidURL) {
				t.Fatalf("error = %v, se esperaba ErrInvalidURL", err)
			}
		})
	}
}

func TestCreateReintentaSiElCodigoExiste(t *testing.T) {
	store := &fakeStore{failures: 2, err: ErrCodeExists}

	if _, err := NewService(store).Create(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if store.calls != 3 {
		t.Errorf("intentos = %d, se esperaban 3", store.calls)
	}
}

func TestCreateFallaAlAgotarIntentos(t *testing.T) {
	store := &fakeStore{failures: maxAttempts, err: ErrCodeExists}

	_, err := NewService(store).Create(context.Background(), "https://example.com")
	if !errors.Is(err, ErrCodeExists) {
		t.Fatalf("error = %v, se esperaba ErrCodeExists", err)
	}
	if store.calls != maxAttempts {
		t.Errorf("intentos = %d, se esperaban %d", store.calls, maxAttempts)
	}
}

func TestCreateNoReintentaOtrosErrores(t *testing.T) {
	errBase := errors.New("base de datos caída")
	store := &fakeStore{failures: 1, err: errBase}

	_, err := NewService(store).Create(context.Background(), "https://example.com")
	if !errors.Is(err, errBase) {
		t.Fatalf("error = %v, se esperaba el error original", err)
	}
	if store.calls != 1 {
		t.Errorf("intentos = %d, se esperaba 1", store.calls)
	}
}

func TestMemoryStoreRechazaCodigoDuplicado(t *testing.T) {
	store := NewMemoryStore()
	link := Link{Code: "abc1234", URL: "https://example.com"}

	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := store.Save(context.Background(), link); !errors.Is(err, ErrCodeExists) {
		t.Fatalf("error = %v, se esperaba ErrCodeExists", err)
	}
}
