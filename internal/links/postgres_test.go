package links

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// newTestPostgres se conecta a TEST_DATABASE_URL y omite la prueba si no está
// definida. Es una variable aparte de DATABASE_URL para no probar sobre datos reales.
func newTestPostgres(t *testing.T) *PostgresStore {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL no está definida")
	}

	store, err := NewPostgresStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("no se pudo conectar: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

// saveTestLink crea un enlace con código aleatorio y lo borra al terminar.
func saveTestLink(t *testing.T, store *PostgresStore) Link {
	t.Helper()

	code, err := generateCode()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	link := Link{Code: code, URL: "https://example.com", CreatedAt: time.Now().UTC()}
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	t.Cleanup(func() {
		if _, err := store.pool.Exec(context.Background(), `DELETE FROM links WHERE code = $1`, code); err != nil {
			t.Errorf("no se pudo limpiar: %v", err)
		}
	})
	return link
}

func TestPostgresGuardaYBusca(t *testing.T) {
	store := newTestPostgres(t)
	link := saveTestLink(t, store)

	found, err := store.Find(context.Background(), link.Code)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if found.URL != link.URL || found.Clicks != 0 || !found.CreatedAt.Equal(link.CreatedAt.Truncate(time.Microsecond)) {
		t.Errorf("enlace inesperado: %+v", found)
	}
}

func TestPostgresRechazaCodigoDuplicado(t *testing.T) {
	store := newTestPostgres(t)
	link := saveTestLink(t, store)

	if err := store.Save(context.Background(), link); !errors.Is(err, ErrCodeExists) {
		t.Fatalf("error = %v, se esperaba ErrCodeExists", err)
	}
}

func TestPostgresDevuelveNotFound(t *testing.T) {
	store := newTestPostgres(t)

	if _, err := store.Find(context.Background(), "noexiste"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find: error = %v, se esperaba ErrNotFound", err)
	}
	if _, err := store.IncrementClicks(context.Background(), "noexiste"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("IncrementClicks: error = %v, se esperaba ErrNotFound", err)
	}
}

func TestPostgresNoPierdeClicsSimultaneos(t *testing.T) {
	const visitas = 50
	store := newTestPostgres(t)
	link := saveTestLink(t, store)

	var wg sync.WaitGroup
	for range visitas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.IncrementClicks(context.Background(), link.Code); err != nil {
				t.Errorf("error inesperado: %v", err)
			}
		}()
	}
	wg.Wait()

	final, err := store.Find(context.Background(), link.Code)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if final.Clicks != visitas {
		t.Errorf("clics = %d, se esperaban %d", final.Clicks, visitas)
	}
}
