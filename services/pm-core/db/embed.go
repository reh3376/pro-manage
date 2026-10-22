// Package dbassets embeds the SQL the pm-core binary carries: goose migrations
// (applied at startup when PM_DB_AUTO_MIGRATE=true, or via `make db-migrate`)
// and the development seed for the fictional Northfield Spirits Co.
package dbassets

import "embed"

// Migrations holds migrations/*.sql in goose format.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Seeds holds seeds/*.sql (development data only).
//
//go:embed seeds/*.sql
var Seeds embed.FS
