package links

import (
	"context"
	"sync"
)

// MemoryStore guarda los enlaces en memoria. Se pierde al reiniciar,
// así que sirve para desarrollo y pruebas, no para producción.
type MemoryStore struct {
	mu    sync.RWMutex
	links map[string]Link
}

// NewMemoryStore crea un almacenamiento vacío.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{links: make(map[string]Link)}
}

// Save guarda el enlace o devuelve ErrCodeExists si el código ya existe.
func (m *MemoryStore) Save(_ context.Context, link Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.links[link.Code]; exists {
		return ErrCodeExists
	}
	m.links[link.Code] = link
	return nil
}

// Find devuelve el enlace o ErrNotFound si el código no existe.
func (m *MemoryStore) Find(_ context.Context, code string) (Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	link, exists := m.links[code]
	if !exists {
		return Link{}, ErrNotFound
	}
	return link, nil
}
