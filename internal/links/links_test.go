package links

import (
	"context"
	"errors"
	"regexp"
	"strings"
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

func (f *fakeStore) Delete(context.Context, string) error {
	return f.err
}

func TestVisitSumaUnClic(t *testing.T) {
	service := NewService(NewMemoryStore())
	creado, err := service.Create(context.Background(), NewLink{URL: "https://example.com"})
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
	creado, err := service.Create(context.Background(), NewLink{URL: "https://example.com"})
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
	creado, err := service.Create(context.Background(), NewLink{URL: "https://example.com"})
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

	link, err := service.Create(context.Background(), NewLink{URL: "  https://example.com/ruta?a=1  "})
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
			if _, err := service.Create(context.Background(), NewLink{URL: url}); !errors.Is(err, ErrInvalidURL) {
				t.Fatalf("error = %v, se esperaba ErrInvalidURL", err)
			}
		})
	}
}

func TestCreateReintentaSiElCodigoExiste(t *testing.T) {
	store := &fakeStore{failures: 2, err: ErrCodeExists}

	if _, err := NewService(store).Create(context.Background(), NewLink{URL: "https://example.com"}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if store.calls != 3 {
		t.Errorf("intentos = %d, se esperaban 3", store.calls)
	}
}

func TestCreateFallaAlAgotarIntentos(t *testing.T) {
	store := &fakeStore{failures: maxAttempts, err: ErrCodeExists}

	_, err := NewService(store).Create(context.Background(), NewLink{URL: "https://example.com"})
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

	_, err := NewService(store).Create(context.Background(), NewLink{URL: "https://example.com"})
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

func TestCreateUsaElAliasPedido(t *testing.T) {
	service := NewService(NewMemoryStore())

	link, err := service.Create(context.Background(), NewLink{URL: "https://example.com", Alias: "mi-promo_2026"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if link.Code != "mi-promo_2026" {
		t.Errorf("código = %q, se esperaba el alias", link.Code)
	}

	encontrado, err := service.Resolve(context.Background(), "mi-promo_2026")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if encontrado.URL != "https://example.com" {
		t.Errorf("URL = %q", encontrado.URL)
	}
}

// Valores límite del largo del alias: el mínimo es 3 y el máximo 30.
func TestCreateAliasLimitesDeLargo(t *testing.T) {
	casos := []struct {
		nombre string
		alias  string
		valido bool
	}{
		{"2 caracteres", "ab", false},
		{"3 caracteres", "abc", true},
		{"30 caracteres", strings.Repeat("a", 30), true},
		{"31 caracteres", strings.Repeat("a", 31), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NewService(NewMemoryStore()).Create(context.Background(), NewLink{URL: "https://example.com", Alias: c.alias})
			if c.valido && err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if !c.valido && !errors.Is(err, ErrInvalidAlias) {
				t.Fatalf("error = %v, se esperaba ErrInvalidAlias", err)
			}
		})
	}
}

func TestCreateRechazaAliasInvalido(t *testing.T) {
	casos := map[string]string{
		"con espacio":        "mi promo",
		"con barra":          "mi/promo",
		"con tilde":          "promoción",
		"con punto":          "mi.promo",
		"ruta api":           "api",
		"ruta health":        "health",
		"ruta en mayúsculas": "HEALTH",
	}

	for nombre, alias := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := NewService(NewMemoryStore()).Create(context.Background(), NewLink{URL: "https://example.com", Alias: alias})
			if !errors.Is(err, ErrInvalidAlias) {
				t.Fatalf("error = %v, se esperaba ErrInvalidAlias", err)
			}
		})
	}
}

func TestCreateValidaLaURLAntesQueElAlias(t *testing.T) {
	_, err := NewService(NewMemoryStore()).Create(context.Background(), NewLink{URL: "ftp://example.com", Alias: "x"})
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("error = %v, se esperaba ErrInvalidURL", err)
	}
}

func TestCreateRechazaAliasOcupadoSinReintentar(t *testing.T) {
	store := &fakeStore{failures: 1, err: ErrCodeExists}

	_, err := NewService(store).Create(context.Background(), NewLink{URL: "https://example.com", Alias: "ocupado"})
	if !errors.Is(err, ErrAliasTaken) {
		t.Fatalf("error = %v, se esperaba ErrAliasTaken", err)
	}
	if store.calls != 1 {
		t.Errorf("intentos = %d, se esperaba 1", store.calls)
	}
}

func TestCreateConAliasConservaErroresDelAlmacenamiento(t *testing.T) {
	errBase := errors.New("base de datos caída")
	store := &fakeStore{failures: 1, err: errBase}

	_, err := NewService(store).Create(context.Background(), NewLink{URL: "https://example.com", Alias: "promo"})
	if !errors.Is(err, errBase) {
		t.Fatalf("error = %v, se esperaba el error original", err)
	}
}

// createForDelete crea un enlace y devuelve su código y su token.
func createForDelete(t *testing.T, service *Service) Link {
	t.Helper()

	link, err := service.Create(context.Background(), NewLink{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return link
}

func TestCreateEntregaTokenYGuardaSoloElHash(t *testing.T) {
	store := NewMemoryStore()
	link := createForDelete(t, NewService(store))

	if len(link.DeleteToken) != tokenLength {
		t.Fatalf("token = %q, se esperaban %d caracteres", link.DeleteToken, tokenLength)
	}
	guardado, err := store.Find(context.Background(), link.Code)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if guardado.DeleteToken != "" {
		t.Error("el token en claro no debería guardarse")
	}
	if guardado.DeleteTokenHash != hashToken(link.DeleteToken) {
		t.Error("el hash guardado no corresponde al token entregado")
	}
}

func TestDeleteConTokenCorrecto(t *testing.T) {
	service := NewService(NewMemoryStore())
	link := createForDelete(t, service)

	if err := service.Delete(context.Background(), link.Code, link.DeleteToken); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if _, err := service.Resolve(context.Background(), link.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, el enlace sigue guardado", err)
	}
}

// Caja blanca: cubre cada condición que rechaza el token en Delete.
func TestDeleteRechazaTokenInvalido(t *testing.T) {
	service := NewService(NewMemoryStore())
	link := createForDelete(t, service)
	otro := createForDelete(t, service)

	casos := map[string]string{
		"vacío":            "",
		"incorrecto":       "no-es-el-token",
		"de otro enlace":   otro.DeleteToken,
		"el hash guardado": hashToken(link.DeleteToken),
	}
	for nombre, token := range casos {
		t.Run(nombre, func(t *testing.T) {
			if err := service.Delete(context.Background(), link.Code, token); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, se esperaba ErrForbidden", err)
			}
		})
	}

	if _, err := service.Resolve(context.Background(), link.Code); err != nil {
		t.Fatalf("el enlace no debería eliminarse: %v", err)
	}
}

func TestDeleteRechazaEnlacesSinToken(t *testing.T) {
	// Un enlace creado antes de esta función tiene el hash vacío.
	store := NewMemoryStore()
	if err := store.Save(context.Background(), Link{Code: "antiguo", URL: "https://example.com"}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	err := NewService(store).Delete(context.Background(), "antiguo", "cualquiera")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, se esperaba ErrForbidden", err)
	}
}

func TestDeleteFallaSiElCodigoNoExiste(t *testing.T) {
	err := NewService(NewMemoryStore()).Delete(context.Background(), "noexiste", "token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}

func TestDeleteConservaErroresDelAlmacenamiento(t *testing.T) {
	errBase := errors.New("base de datos caída")

	err := NewService(&fakeStore{err: errBase}).Delete(context.Background(), "abc1234", "token")
	if !errors.Is(err, errBase) {
		t.Fatalf("error = %v, se esperaba el error original", err)
	}
}
