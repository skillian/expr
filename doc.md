# Package `expr`

The `expr` package allows arbitrary logical/mathmatical expressions to
be expressed as data and in a type-safe way.

It is intended to be a Go-like implementation of C#'s
`System.Linq.Expressions`.

## Why?

### 1. Building expressions at runtime

If you need to build expressions to query for and/or display data
where the results or the data displayed in the results is only known
at the time the queries are executed, this may be for you.

### 2. Passing expressions to external repositories

This package is meant for expressions to be executed by remote
data sources, such as database engines or other external processes and
to "fall back" to local execution.

**Note**: If you don't want an expression to execute remotely, then just
write it as a function!

### 3. Handling disparate underlying data sources

SQL is fairly standarized, but (minor) adjustments must be made between
different "dialects," such as Microsoft SQL Server, Oracle, Postgres,
MySQL, SQLite, etc.  If you want to code your query once and have it
work across different platforms, this may be for you.

### 4. Mixing remotely- and locally-executed queries:

Perhaps you have executed a query to obtain some results.  If you then
have to add an additional constraint to the query, rather than add that
constraint and re-execute the query against the main data source,
perhaps it would be better to simply filter the results you already
have.  This package can allow you to do that.