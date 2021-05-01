package expr

import (
	"context"
	"sync"

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
func AddValuesToContext(ctx context.Context, vs Values) context.Context {
	if vs2, ok := ValuesFromContextOK(ctx); ok && vs2 == vs {
		return ctx
	}
	return context.WithValue(ctx, valuesContextKey{}, vs)
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

// ValuesFromContextOrNew attempts to retrieve an existing collection of values
// from a context or else creates a new collection and adds them to the
// context.
func ValuesFromContextOrNew(ctx context.Context) (context.Context, Values) {
	vs, ok := ValuesFromContextOK(ctx)
	if !ok {
		vs = NewValues()
		ctx = AddValuesToContext(ctx, vs)
	}
	return ctx, vs
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

type concurrentValues struct {
	locker
	values Values
}

// ConcurrentValues creates a collection of values safe for concurrent use.
func ConcurrentValues(lr sync.Locker, pairs ...VarValuePair) Values {
	return concurrentValues{lockerOf(lr), NewValues(pairs...)}
}

func (vs concurrentValues) Get(v Var) (x interface{}) {
	vs.locker.RLock()
	x = vs.values.Get(v)
	vs.locker.RUnlock()
	return
}

func (vs concurrentValues) Len() int { return vs.values.Len() }

func (vs concurrentValues) Set(v Var, x interface{}) {
	vs.locker.Lock()
	vs.values.Set(v, x)
	vs.locker.Unlock()
}

type locker interface {
	sync.Locker
	RLock()
	RUnlock()
}

func lockerOf(s sync.Locker) locker {
	if lr, ok := s.(locker); ok {
		return lr
	}
	return dummyRlocker{s}
}

type dummyRlocker struct {
	sync.Locker
}

func (d dummyRlocker) RLock()   { d.Locker.Lock() }
func (d dummyRlocker) RUnlock() { d.Locker.Unlock() }
