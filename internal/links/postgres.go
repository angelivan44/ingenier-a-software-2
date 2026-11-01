package links

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxConnections es bajo porque cada instancia serverless abre su propio pool
// y el plan gratuito de la base de datos admite pocas conexiones.
const maxConnections = 3

const createTable = `
CREATE TABLE IF NOT EXISTS links (
	code       TEXT PRIMARY KEY,
	url        TEXT NOT NULL,
	clicks     BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// PostgresStore guarda los enlaces en Postgres, así sobreviven a los
// reinicios y se comparten entre instancias.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore se conecta a la base de datos y crea la tabla si falta.
func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("leer DATABASE_URL: %w", err)
	}
	config.MaxConns = maxConnections
	// Sin sentencias preparadas: los poolers de conexiones no las conservan
	// entre peticiones.
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("crear pool: %w", err)
	}
	if _, err := pool.Exec(ctx, createTable); err != nil {
		pool.Close()
		return nil, fmt.Errorf("crear tabla: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Close libera las conexiones.
func (p *PostgresStore) Close() {
	p.pool.Close()
}

// Save guarda el enlace o devuelve ErrCodeExists si el código ya existe.
func (p *PostgresStore) Save(ctx context.Context, link Link) error {
	result, err := p.pool.Exec(ctx,
		`INSERT INTO links (code, url, clicks, created_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (code) DO NOTHING`,
		link.Code, link.URL, link.Clicks, link.CreatedAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrCodeExists
	}
	return nil
}

// Find devuelve el enlace o ErrNotFound si el código no existe.
func (p *PostgresStore) Find(ctx context.Context, code string) (Link, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT code, url, clicks, created_at FROM links WHERE code = $1`, code)
	return scanLink(row)
}

// IncrementClicks suma un clic en una sola consulta: Postgres bloquea la fila,
// así dos visitas simultáneas no se pisan.
func (p *PostgresStore) IncrementClicks(ctx context.Context, code string) (Link, error) {
	row := p.pool.QueryRow(ctx,
		`UPDATE links SET clicks = clicks + 1 WHERE code = $1
		 RETURNING code, url, clicks, created_at`, code)
	return scanLink(row)
}

func scanLink(row pgx.Row) (Link, error) {
	var link Link
	err := row.Scan(&link.Code, &link.URL, &link.Clicks, &link.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, err
	}
	link.CreatedAt = link.CreatedAt.UTC()
	return link, nil
}
