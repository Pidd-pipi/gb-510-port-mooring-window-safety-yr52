package service

import (
	"database/sql"
	"strings"
)

// sqlSerializableLevel returns SERIALIZABLE on PostgreSQL/MySQL. SQLite only
// supports a single effective isolation level (its write transactions are
// serialized by the database file lock), so nil keeps the driver default.
func sqlSerializableLevel(dialect string) sql.IsolationLevel {
	switch dialect {
	case "postgres", "mysql":
		return sql.LevelSerializable
	default:
		return sql.LevelDefault
	}
}

func isolationOrDefault(level sql.IsolationLevel) *sql.TxOptions {
	if level == sql.LevelDefault {
		return nil
	}
	return &sql.TxOptions{Isolation: level}
}

func setIsolationSQL(dialect string) string {
	if dialect == "mysql" {
		return "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE"
	}
	return "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE"
}

// isSerializationFailure recognizes SQLSTATE 40001 / 40P01 on Postgres,
// 1213/1205 on MySQL and the SQLite "database is locked" signal produced by
// concurrent single-writer transactions.
func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "40001") || strings.Contains(message, "40p01") ||
		strings.Contains(message, "serialization failure") || strings.Contains(message, "deadlock") {
		return true
	}
	if strings.Contains(message, "1213") || strings.Contains(message, "1205") || strings.Contains(message, "try restarting transaction") {
		return true
	}
	return strings.Contains(message, "database is locked") || strings.Contains(message, "sqlite_busy")
}
