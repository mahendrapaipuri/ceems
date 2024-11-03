// Package migrator implements database migrations
package migrator

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrator implements DB migrations.
type Migrator struct {
	logger    *slog.Logger
	srcDriver source.Driver
}

// New returns new instance of Migrator.
func New(sqlFiles embed.FS, dirName string, logger *slog.Logger) (*Migrator, error) {
	d, err := iofs.New(sqlFiles, dirName)
	if err != nil {
		return nil, err
	}

	return &Migrator{
		logger:    logger,
		srcDriver: d,
	}, nil
}

// ApplyMigrations applies DB migrations.
func (m *Migrator) ApplyMigrations(db *sql.DB) error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("unable to create db instance: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", m.srcDriver, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("unable to create migration: %w", err)
	}

	m.logger.Info("Applying DB migrations")

	if err = migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("unable to apply migrations %w", err)
	}

	if version, dirty, err := migrator.Version(); err != nil {
		m.logger.Error("Failed to get DB migration version", "err", err)
	} else {
		m.logger.Debug("Current DB migration version", "version", version, "dirty", dirty)
	}

	return nil
}
