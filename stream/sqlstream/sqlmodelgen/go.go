package sqlmodelgen

import (
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
)

var (
	GoModelContext ModelContext = goModelContext{}

	//go:embed go/*.txt
	goFs embed.FS

	goModelFs fs.FS = func(fsys fs.FS) fs.FS {
		fsys, err := fs.Sub(goFs, "go")
		if err != nil {
			panic(err)
		}
		return fsys
	}(goFs)
)

// GoModelContext implements the ModelContext interface for the Go
// programming language.
type goModelContext struct{}

func (goModelContext) FS() fs.FS { return goModelFs }

// ModelType produces Go data types from sqltype.Type definitions.
func (goModelContext) ModelType(t sqltypes.Type) (namespace, typename string, err error) {
	switch t := t.(type) {
	case sqltypes.BoolType:
		return "", "bool", nil
	case sqltypes.IntType:
		switch {
		case t.Bits <= 8:
			return "", "int8", nil
		case t.Bits <= 16:
			return "", "int16", nil
		case t.Bits <= 32:
			return "", "int32", nil
		case t.Bits <= 64:
			return "", "int64", nil
		}
		return "", "", errors.Errorf1(
			"int with %d bits not supported",
			t.Bits)
	case sqltypes.FloatType:
		switch {
		case t.Mantissa <= 24:
			return "", "float32", nil
		case t.Mantissa <= 53:
			return "", "float64", nil
		}
		return "", "", errors.Errorf1(
			"float with %d mantissa bits not "+
				"supported", t.Mantissa)
	case sqltypes.StringType:
		return "", "string", nil
	case sqltypes.TimeType:
		return "time", "time.Time", nil
	case sqltypes.BytesType:
		return "", "[]byte", nil
	}
	return "", "interface{}", nil
}

func (goModelContext) EnsureNamespaces(c *Config) []string {
	nss := make([]string, 1, 2)
	nss[0] = "github.com/skillian/expr/stream/sqlstream/sqltypes"
dbLoop:
	for _, db := range c.Databases {
		for _, sch := range db.Schemas {
			for _, tbl := range sch.Tables {
				if tbl.PK != nil || tbl.Key != nil {
					nss = append(nss, "github.com/skillian/expr/stream/sqlstream")
					break dbLoop
				}
			}
		}
	}
	return nss
}

func (goModelContext) OrganizeNamespaces(nss []string) []string {
	stdlib := make([]string, 0, len(nss))
	external := make([]string, 0, len(nss))
	for _, ns := range nss {
		pivot := strings.IndexByte(ns, '/')
		if pivot == -1 {
			stdlib = append(stdlib, ns)
			continue
		}
		firstPart := ns[:pivot]
		if strings.ContainsRune(firstPart, '.') {
			// assume it's a host name and therefore external
			external = append(external, ns)
			continue
		}
		// otherwise, it's a deep path to a stdlib
		stdlib = append(stdlib, ns)
	}
	sort.Strings(stdlib)
	sort.Strings(external)
	nss = nss[:0]
	if len(stdlib) > 0 {
		nss = append(nss, stdlib...)
		if len(external) > 0 {
			nss = append(nss, "") // gap
		}
	}
	return append(nss, external...)
}

