// Package store persiste la cola de jobs en SQLite local.
//
// Usa modernc.org/sqlite (driver puro Go, sin CGO) para que el .exe pueda
// cross-compilarse desde Linux y siga siendo estático en Windows.
//
// Diseño:
//
//   - Una sola conexión escritora ([sql.DB.SetMaxOpenConns](1)). Evita
//     SQLITE_BUSY a costa de throughput, lo cual es totalmente aceptable
//     para una cola local que procesa decenas de jobs por minuto, no miles.
//
//   - WAL mode + busy_timeout=5000ms. WAL mejora la concurrencia entre
//     escritores y lectores en el mismo proceso; busy_timeout reintenta
//     automáticamente cuando algún otro proceso (poco frecuente) tiene la DB.
//
//   - Migraciones idempotentes: el schema se aplica con CREATE TABLE IF
//     NOT EXISTS en cada arranque. No hay sistema de versiones porque el
//     schema es chico y estable.
//
//   - Recovery de jobs huérfanos: [RecoverStaleJobs] revierte a "queued"
//     todos los jobs que quedaron como "printing" cuando el proceso anterior
//     murió (crash, kill, reinicio del SO). El worker los reintenta normalmente.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusPrinting  Status = "printing"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }
