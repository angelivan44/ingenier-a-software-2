package links

import (
	"context"
	"errors"
	"regexp"
	"sync"
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

func (f *fakeStore) Find(context.Context, string) (Link, error) {
	return Link{}, f.err
}

func (f *fakeStore) IncrementClicks(context.Context, string) (Link, error) {
	return Link{}, f.err
}

func TestVisitSumaUnClic(t *testing.T) {
	service := NewService(NewMemoryStore())
	creado, err := service.Create(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	for clics := int64(1); clics <= 2; clics++ {
		visitado, err := service.Visit(context.Background(), creado.Code)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if visitado.Clicks != clics {
			t.Errorf("clics = %d, se esperaban %d", visitado.Clicks, clics)
		}
	}
}

func TestVisitNoPierdeClicsSimultaneos(t *testing.T) {
	const visitas = 100
	service := NewService(NewMemoryStore())
	creado, err := service.Create(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var wg sync.WaitGroup
	for range visitas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.Visit(context.Background(), creado.Code); err != nil {
				t.Errorf("error inesperado: %v", err)
			}
		}()
	}
	wg.Wait()

	final, err := service.Resolve(context.Background(), creado.Code)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if final.Clicks != visitas {
		t.Errorf("clics = %d, se esperaban %d", final.Clicks, visitas)
	}
}

func TestVisitFallaSiElCodigoNoExiste(t *testing.T) {
	_, err := NewService(NewMemoryStore()).Visit(context.Background(), "noexiste")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}

func TestVisitConservaErroresDelAlmacenamiento(t *testing.T) {
	errBase := errors.New("base de datos caída")

	_, err := NewService(&fakeStore{err: errBase}).Visit(context.Background(), "abc1234")
	if !errors.Is(err, errBase) {
		t.Fatalf("error = %v, se esperaba el error original", err)
	}
}

func TestResolveDevuelveElEnlaceCreado(t *testing.T) {
	service := NewService(NewMemoryStore())
	creado, err := service.Create(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	encontrado, err := service.Resolve(context.Background(), creado.Code)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if encontrado.URL != "https://example.com" {
		t.Errorf("URL = %q", encontrado.URL)
	}
}

func TestResolveFallaSiElCodigoNoExiste(t *testing.T) {
	_, err := NewService(NewMemoryStore()).Resolve(context.Background(), "noexiste")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}

func TestResolveConservaErroresDelAlmacenamiento(t *testing.T) {
	errBase := errors.New("base de datos caída")

	_, err := NewService(&fakeStore{err: errBase}).Resolve(context.Background(), "abc1234")
	if !errors.Is(err, errBase) {
		t.Fatalf("error = %v, se esperaba el error original", err)
	}
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
