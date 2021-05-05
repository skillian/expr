package sqlmodelgen

import (
	"embed"
	"io/fs"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
)

var (
	CSModelContext ModelContext = csModelContext{}

	//go:embed cs/*.txt
	csFs embed.FS

	csModelFs fs.FS = func(fsys fs.FS) fs.FS {
		fsys, err := fs.Sub(csFs, "cs")
		if err != nil {
			panic(err)
		}
		return fsys
	}(csFs)
)

type csModelContext struct{}

func (csModelContext) FS() fs.FS { return csModelFs }

func (csModelContext) ModelType(t sqltypes.Type) (namespace, typename string, err error) {
	switch t := t.(type) {
	case sqltypes.BoolType:
		return "", "bool", nil
	case sqltypes.IntType:
		switch {
		case t.Bits <= 8:
			return "", "byte", nil
		case t.Bits <= 16:
			return "", "short", nil
		case t.Bits <= 32:
			return "", "int", nil
		case t.Bits <= 64:
			return "", "long", nil
		}
		return "", "", errors.Errorf1(
			"int with %d bits not supported",
			t.Bits)
	case sqltypes.FloatType:
		switch {
		case t.Mantissa <= 24:
			return "", "float", nil
		case t.Mantissa <= 53:
			return "", "double", nil
		}
		return "", "", errors.Errorf1(
			"float with %d mantissa bits not "+
				"supported", t.Mantissa)
	case sqltypes.Nullable:
		ns, tn, err := CSModelContext.ModelType(t[0])
		return ns, tn + "?", err
	case sqltypes.StringType:
		return "", "string", nil
	case sqltypes.TimeType:
		return "time", "DateTime", nil
	case sqltypes.BytesType:
		return "", "byte[]", nil
	}
	return "", "object", nil
}
