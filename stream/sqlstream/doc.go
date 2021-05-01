// Package sqlstream implements the stream package's interfaces by translating
// the expressions into SQL queries.
//
// Models define rows of data in result sets, such as those from tables, views,
// stored procedures, functions, etc., but are not bound to their data source.
// When you define a table, you pass in a model to define its data structure.
// When you query that table, you get that data structure out, but you could
// then use that structure when storing into another table, as long as the
// models are compatible in field names and types.
//
// Some result sets (such as those selected from joins or window functions)
// produce models that are composed of "submodels."  This package is meant to
// handle that.
package sqlstream
