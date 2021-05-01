package config

type Config struct {
	Namespace      string
	Databases      map[string]Database
	DatabaseNamers Namers
}

type Namers struct {
	SQLNamer   string
	ModelNamer string
}

type Database struct {
	Schemas map[string]Schema
	Namers  struct {
		Table   Namers
		IDType  Namers
		KeyType Namers
		Column  Namers
		Schema  Namers
	}
}

type Schema struct {
	Tables map[string]Table
	Views  map[string]View
}

type Table struct {
	// PK is a comma-separated list of column names that uniquely
	// identify records in this table.
	Columns map[string]Column
}

type Column struct {
	// PK is true if the column is a primary key (or a component
	// of a primary key if multiple columns in the same table have
	// PK = true).
	PK bool

	// FK is filled in with the dot-separated table.column of the
	// primary key that this FK refers to.  If referencing a table
	// in another schema, use schema.table.column or if another
	// database, database.schema.table.column.
	FK   string
	Type string
}

type View Table
