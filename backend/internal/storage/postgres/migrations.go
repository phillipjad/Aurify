package postgres

//go:generate sqlc generate -f ../../../sqlc.yaml

import "embed"

// migrationsFS holds the goose SQL migrations applied on startup by Connect.
// Embedding them keeps the schema versioned alongside the binary so a fresh
// database is brought up to date automatically (see Connect in client.go).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS
