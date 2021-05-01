package sqlstream

import (
	"context"
	"database/sql"
)

type transactionContextKey struct{}

// TxFromContext extracts a *sql.Tx from a context.
func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(*sql.Tx)
	return tx, ok
}

// AddTxToContext adds a *sql.Tx to the context if it isn't already in
// the context.
func AddTxToContext(ctx context.Context, tx *sql.Tx) context.Context {
	if tx2, ok := TxFromContext(ctx); ok && tx == tx2 {
		return ctx
	}
	return context.WithValue(ctx, transactionContextKey{}, tx)
}
