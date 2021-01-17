package sqlstream

import (
	"context"
	"database/sql"
	"io"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/internal"
	"github.com/skillian/expr/stream"
)

// Query a model in the database
func (db *DB) Query(ctx context.Context, v interface{}, options ...stream.QueryOption) (stream.Streamer, error) {
	m, err := ModelOf(v)
	if err != nil {
		return nil, err
	}
	qos, err := stream.NewQueryOptions(options...)
	if err != nil {
		return nil, errors.Errorf1From(
			err, "failed to create query options from %v", options)
	}
	q := &query{
		dbcmd: dbcmd{db: db},
		from:  db.getTable(v, m),
		//to:    m,
	}
	q.base = q
	if limit, ok := qos.Limit(); ok {
		q.dbcmd.limit = limit
	}
	q.initSQLFunc()
	return q, nil
}

type query struct {
	base *query
	dbcmd
	from *table
	//to      Model
	where   expr.Expr
	joins   []queryJoin
	sqlOnce sync.Once
	sqlFunc func()
	sqlStr  string
	sqlArgs []queryArg
	sqlVars []queryVar
}

type queryArg struct {
	v interface{}
	i int
}

type queryVar struct {
	v expr.Var
	i int
}

type queryJoin struct {
	*query
	when expr.Expr
	then expr.Expr
}

func (q *query) initSQLFunc() {
	q.sqlFunc = func() {
		qb := newQueryBuilder(q)
		q.sqlStr = qb.buildSQL()
		q.sqlArgs = qb.sqlArgs
		q.sqlVars = qb.sqlVars
		logger.Verbose3(
			"Created query:\n\n%s\n\nArgs:\n\n%#v\n\nVars:\n\n%#v",
			q.sqlStr, q.sqlArgs, q.sqlVars)
	}
}

func (q *query) clone() *query {
	q2 := &query{
		base:    q.base,
		dbcmd:   q.dbcmd,
		from:    q.from,
		where:   q.where,
		joins:   append(make([]queryJoin, 0, internal.CapForLen(len(q.joins))), q.joins...),
		sqlArgs: append(make([]queryArg, 0, internal.CapForLen(len(q.sqlArgs))), q.sqlArgs...),
		sqlVars: append(make([]queryVar, 0, internal.CapForLen(len(q.sqlVars))), q.sqlVars...),
	}
	q2.initSQLFunc()
	return q2
}

func (q *query) checkVars(e expr.Expr, what string) (local bool) {
	// invert the result of Inspect.
	return !expr.Inspect(e, func(e expr.Expr) bool {
		const prefix = "Using local %s because "
		if e == nil {
			return true
		}
		if va, ok := e.(expr.Var); ok {
			q2, ok := va.(*query)
			if !ok {
				logger.Info2(
					prefix+"var %v is not a SQL var",
					what, va)
				return false
			}
			if q2.db != q.db {
				logger.Info3(
					prefix+"%v is for a different db "+
						"than %v",
					what, q2, q)
				return false
			}
		}
		return true
	})
}

func (q *query) Filter(e expr.Expr) (stream.Streamer, error) {
	if q.checkVars(e, "filter") {
		return stream.NewLocalFilter(q, e)
	}
	if q.where != nil {
		e = expr.And{q.where, e}
	}
	q2 := q.clone()
	q2.where = e
	return q2, nil
}

func (q *query) Join(other stream.Streamer, when, then expr.Expr, options ...stream.JoinOption) (stream.Streamer, error) {
	oq, ok := other.(*query)
	if !ok || q.checkVars(when, "join") || q.checkVars(then, "join") {
		return stream.NewLocalJoiner(q, other, when, then)
	}
	q2 := q.clone()
	if oq.where != nil {
		if q2.where != nil {
			q2.where = oq.where
		}
		q2.where = expr.And{q2.where, oq.where}
	}
	q2.joins = append(q2.joins, queryJoin{query: oq, when: when, then: then})
	q2.joins = append(q2.joins, oq.joins...)
	return q2, nil
}

func (q *query) Stream(ctx context.Context) (stream.Stream, error) {
	q.sqlOnce.Do(q.sqlFunc)
	var args []interface{}
	length := len(q.sqlArgs) + len(q.sqlVars)
	if length > 0 {
		args = make([]interface{}, length)
		if len(q.sqlVars) > 0 {
			vs, err := expr.ValuesFromContext(ctx)
			if err != nil {
				return nil, errors.Errorf0From(
					err, "query has variables but no values were "+
						"provided in the context")
			}
			for _, sv := range q.sqlVars {
				args[sv.i] = vs.Get(sv.v)
			}
		}
		for _, sa := range q.sqlArgs {
			args[sa.i] = sa.v
		}
	}
	rows, err := q.db.DB.QueryContext(ctx, q.sqlStr, args...)
	if err != nil {
		return nil, errors.Errorf2From(
			err, "query failed:\nSQL:\n\t%s\nArgs:\n\t%#v",
			q.sqlStr, args)
	}
	return &queryRows{q: q, rows: rows}, nil
}

func (q *query) Var() expr.Var { return q.base }

type queryRows struct {
	q      *query
	rows   *sql.Rows
	fields []interface{}
}

func (qr *queryRows) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if !qr.rows.Next() {
		return io.EOF
	}
	v := vs.Get(qr.Var())
	if v == nil {
		return errors.Errorf(
			"Query results require a model to scan into")
	}
	m, err := ModelOf(v)
	if err != nil {
		return errors.Errorf1From(
			err, "cannot scan into %v", v)
	}
	qr.fields = m.AppendFields(qr.fields[:0])
	logger.Verbose1("scanning into %v", spew.Sdump(qr.fields))
	return qr.rows.Scan(qr.fields...)
}

func (qr *queryRows) Var() expr.Var { return qr.q.Var() }

func (qr *queryRows) Close() error {
	return qr.rows.Close()
}
