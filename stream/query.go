package stream

import "context"

// Querier is implemented by streamers that can be queried.  Streamers returned
// by Query should at least implement Filterer to constrain the results coming
// from the queried data source.
type Querier interface {
	Query(ctx context.Context, model interface{}, options ...QueryOption) (Streamer, error)
}

// QueryOptions holds common query options shared between various queriables
type QueryOptions struct {
	// limit holds an optional limit to the number of records to affect.
	// The default 0 value means no limit.
	limit int64

	// customData holds data specific to the Querier implementation.
	customData interface{}
}

// NewQueryOptions creates a QueryOptions struct from the given set of
// configuration options.
func NewQueryOptions(options ...QueryOption) (*QueryOptions, error) {
	qos := &QueryOptions{}
	for _, opt := range options {
		if err := opt(qos); err != nil {
			return nil, err
		}
	}
	return qos, nil
}

// Limit defines the number of results to limit.
func (q *QueryOptions) Limit() (value int64, hasValue bool) { return q.limit, q.limit > 0 }

// CustomData returns optional custom data specific to a query implementation
func (q *QueryOptions) CustomData() interface{} { return q.customData }

// QueryOption configures a query option
type QueryOption func(q *QueryOptions) error

// QueryLimit configures a limit for the query.  Values <= 0 remove the limit.
func QueryLimit(v int64) QueryOption {
	return func(q *QueryOptions) error {
		q.limit = v
		return nil
	}
}

// QueryCustomData specifies custom query configuration data to the query
// options.
func QueryCustomData(data interface{}) QueryOption {
	return func(q *QueryOptions) error {
		q.customData = data
		return nil
	}
}
