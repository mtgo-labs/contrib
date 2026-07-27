package sqlite

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)


const migrationsTable = "mtgo_migrations"
type migration struct {
	repo    string
	version int
	sql     string
}

// driver manages a single SQLite database connection with migration support
// and deferred-write batching.
type driver struct {
	db *sql.DB

	mu       sync.Mutex
	loaded   bool
	closed   bool
	migrated map[string]int

	onLoad []func() error
}

// openDriver opens or creates the SQLite database at path and applies all
// registered migrations.
func openDriver(path string) (*driver, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	d := &driver{
		db:       db,
		migrated: make(map[string]int),
	}

	if err := d.ensureMigrationsTable(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func (d *driver) ensureMigrationsTable() error {
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS ` + migrationsTable + ` (
		repo TEXT NOT NULL PRIMARY KEY,
		version INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("sqlite: create migrations table: %w", err)
	}
	return nil
}


// load opens the database, applies pending migrations, and runs onLoad callbacks.
func (d *driver) load() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.loaded {
		return nil
	}
	d.loaded = true

	// Check existing migration versions
	rows, err := d.db.Query(`SELECT repo, version FROM ` + migrationsTable)
	if err != nil {
		return fmt.Errorf("sqlite: query migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var repo string
		var version int
		if err := rows.Scan(&repo, &version); err != nil {
			return fmt.Errorf("sqlite: scan migration: %w", err)
		}
		d.migrated[repo] = version
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iter migrations: %w", err)
	}

	// Run onLoad callbacks (prepared statements, etc.)
	for _, cb := range d.onLoad {
		if err := cb(); err != nil {
			return err
		}
	}

	return nil
}

func (d *driver) registerOnLoad(cb func() error) {
	d.onLoad = append(d.onLoad, cb)
}

// applyMigrations applies pending migrations. Called before onLoad.
func (d *driver) applyMigrations(migs []migration) error {
	for _, m := range migs {
		current := d.migrated[m.repo]
		if current >= m.version {
			continue
		}
		if _, err := d.db.Exec(m.sql); err != nil {
			return fmt.Errorf("sqlite: migration %s v%d: %w", m.repo, m.version, err)
		}
		if _, err := d.db.Exec(
			`INSERT OR REPLACE INTO `+migrationsTable+` (repo, version) VALUES (?, ?)`,
			m.repo, m.version,
		); err != nil {
			return fmt.Errorf("sqlite: record migration %s v%d: %w", m.repo, m.version, err)
		}
		d.migrated[m.repo] = m.version
	}
	return nil
}

// close closes the database connection.
func (d *driver) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.db.Close()
}
