// Package stream describes sequences of operations to perform on streams of
// objects.  The operations are described by the parent expr package.  While
// streams can be created and used locally, they are meant to describe queries
// to external systems.  If external systems fail to implement some
// stream functionality (e.g. support for filtering is built in but mapping
// is not), those stream steps are executed locally.
package stream
