package postgres

import (
	"errors"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// isRetryableConflict reports deadlocks and serialization failures. PostgreSQL aborts
// one of the competing transactions, which can safely run again from the start.
func isRetryableConflict(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) &&
		(pgErr.Code == "40001" || pgErr.Code == "40P01")
}

// isUnavailable reports failures caused by a temporarily unavailable database.
func isUnavailable(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		code := pgErr.Code

		return isRetryableConflict(err) ||
			strings.HasPrefix(code, "08") || // connection exception
			strings.HasPrefix(code, "53") || // insufficient resources
			code == "55P03" || // lock not available
			code == "57014" || // query canceled by statement timeout
			code == "57P01" || // admin shutdown
			code == "57P02" || // crash shutdown
			code == "57P03" // cannot connect now
	}

	return pgconn.Timeout(err) ||
		pgconn.SafeToRetry(err) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// isUniqueViolation reports whether PostgreSQL rejected a unique constraint.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505"
}

// sqlState returns the SQLSTATE of a PostgreSQL error, or an empty string.
func sqlState(err error) string {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		return pgErr.Code
	}

	return ""
}
