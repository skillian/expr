package sqlmodelgen

import (
	"io/fs"

	//"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
)

// ModelContext produces template fill-ins for model-specific data
type ModelContext interface {
	// ModelType translates a sqltypes.Type into a data type.
	ModelType(t sqltypes.Type) (namespace, typename string, err error)

	// FS returns the directory of templates that should be used
	// unless overridden by a command line parameter
	FS() fs.FS
}

// NamespaceEnsurer is an optional interface that ModelContexts can implement
// to inspect the initialized configuration and return namespaces that must
// exist in the generated templates.
type NamespaceEnsurer interface {
	EnsureNamespaces(c *Config) []string
}

// NamespaceOrganizer is an optional interface that ModelContexts can implement
// to organize the namespaces of the files they generate (e.g. sort them,
// group them, etc.)
type NamespaceOrganizer interface {
	// OrganizeNamespaces receives an unordered collection of namespaces
	// and must return the order of the namespaces as they should appear
	// in the output file.  Blank namespaces can be inserted to create
	// gaps (newlines, for most models) in the namespaces.
	OrganizeNamespaces(ns []string) []string
}
