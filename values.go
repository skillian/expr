package expr

import (
	"context"

	"github.com/skillian/expr/errors"
)

// Values maps variables with values.  The same compiled functions can
// be executed concurrently so the values have to be abstracted away
// from their variables.
type Values interface {
	// Len gets the number of values defined
	Len() int

	// Get the value associated with the Var.
	Get(v Var) interface{}

	// Set the value associated with a Var.
	Set(v Var, value interface{})
}

type valuesImpl struct {
	pairs []VarValuePair
	m     map[Var]int
}

var _ Values = (*valuesImpl)(nil)

type valuesContextKey struct{}

// AddValuesToContext adds the collection of values to the context.
func AddValuesToContext(ctx context.Context, vs Values) (out context.Context, added bool) {
	vs2, ok := ValuesFromContextOK(ctx)
	if ok && vs2 == vs {
		return ctx, false
	}
	return context.WithValue(ctx, valuesContextKey{}, vs), true
}

// ValuesFromContextOK attempts to retrieve the values out of a context and
// returns false if the values were not found.
func ValuesFromContextOK(ctx context.Context) (vs Values, ok bool) {
	vs, ok = ctx.Value(valuesContextKey{}).(Values)
	return
}

// ValuesFromContext attempts to get the values out of a context and returns
// an error if the values cannot be found.
func ValuesFromContext(ctx context.Context) (Values, error) {
	if vs, ok := ValuesFromContextOK(ctx); ok {
		return vs, nil
	}
	return nil, errors.Errorf1("failed to get values from context %+v", ctx)
}

// NewValues creates a new collection of values
func NewValues(pairs ...VarValuePair) Values {
	vs := &valuesImpl{
		pairs: append(make([]VarValuePair, 0, cap(pairs)), pairs...),
		m:     make(map[Var]int, cap(pairs)),
	}
	for i, p := range pairs {
		vs.m[p.Var] = i
	}
	return vs
}

func (vs *valuesImpl) Len() int { return len(vs.pairs) }

// Get a value associated with a variable out of the Values.
func (vs *valuesImpl) Get(v Var) interface{} {
	i, ok := vs.m[v]
	if !ok {
		return nil
	}
	return vs.pairs[i].Value
}

// Set a value in the Values associated with the var.
func (vs *valuesImpl) Set(v Var, value interface{}) {
	i, ok := vs.m[v]
	if ok {
		vs.pairs[i].Value = value
		return
	}
	i = len(vs.pairs)
	vs.pairs = append(vs.pairs, VarValuePair{Var: v, Value: value})
	vs.m[v] = i
	return
}

// Pairs returns a reference to the pairs wrapped inside of the Values
// implementation
func (vs *valuesImpl) Pairs() []VarValuePair { return vs.pairs }

// VarValuePair as its name implies, pairs together a Var and its Value.
type VarValuePair struct {
	Var   Var
	Value interface{}
}

type noValues struct{}

// NoValues holds no values.
var NoValues Values = noValues{}

func (noValues) Len() int                 { return 0 }
func (noValues) Get(v Var) interface{}    { return nil }
func (noValues) Set(v Var, x interface{}) {}
