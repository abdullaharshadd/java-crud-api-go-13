package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaStatements create the tables Hibernate (ddl-auto) generated for the
// User entity and its GenerationType.AUTO id sequence.
var schemaStatements = []string{
	"CREATE TABLE IF NOT EXISTS `user` (" +
		"user_id INT NOT NULL, " +
		"user_name VARCHAR(255) NULL, " +
		"user_email VARCHAR(255) NULL, " +
		"user_password VARCHAR(255) NULL, " +
		"user_role VARCHAR(255) NULL, " +
		"user_about VARCHAR(500) NULL, " +
		"PRIMARY KEY (user_id), " +
		"UNIQUE KEY uk_user_email (user_email)" +
		") ENGINE=InnoDB",
	"CREATE TABLE IF NOT EXISTS hibernate_sequence (next_val BIGINT NULL) ENGINE=InnoDB",
	"INSERT INTO hibernate_sequence (next_val) SELECT 1 FROM DUAL " +
		"WHERE NOT EXISTS (SELECT 1 FROM hibernate_sequence)",
}

// EnsureSchema creates the application's tables if they do not yet exist.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range schemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("repository: ensure schema: %w", err)
		}
	}
	return nil
}
