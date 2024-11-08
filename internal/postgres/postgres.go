package postgres

import (
	"log"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type DB struct {
	*sqlx.DB
}

func NewDB(config *config.Configuration) (*DB, error) {
	dsn := config.Postgres.GetDSN()
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) Close() {
	if err := db.DB.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
}
