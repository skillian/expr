package sqlmodelgen

import (
	"strings"
	"text/template"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
)

// Pair bundles together two arbitrary values.  It is intended to be
// used from the Dict type.
type Pair [2]interface{}

// pair is added to the template's funcmap with the AddFuncs function.
func pair(a, b interface{}) Pair { return Pair{a, b} }

// Dict maps an arbitrary key to a value in templates.
type Dict map[interface{}]interface{}

// dict is added to a template's funcmap with the AddFuncs function
// so that templates can define parameter mappings.
func dict(pairs ...Pair) (d Dict) {
	d = make(Dict, len(pairs))
	for _, p := range pairs {
		d[p[0]] = p[1]
	}
	return
}

// CreateDynTemplate creates a "dyntemplate" function
func CreateDynTemplate(t *template.Template) (dyntemplate func(name string, data interface{}) (string, error)) {
	return func(name string, data interface{}) (string, error) {
		var b strings.Builder
		if err := t.ExecuteTemplate(&b, name, data); err != nil {
			return "", errors.Errorf1From(
				err, "error while executing dynamic "+
					"template: %q", name)
		}
		return b.String(), nil
	}
}

// AddFuncs adds sqlmodelgen's template functions to a FuncMap.
// It will not overwrite existing keys.
func AddFuncs(t *template.Template, m template.FuncMap, mc ModelContext) *template.Template {
	add := func(m template.FuncMap, k string, v interface{}) {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
		return
	}
	if _, ok := m["dyntemplate"]; !ok {
		add(m, "dyntemplate", CreateDynTemplate(t))
	}
	add(m, "pair", pair)
	add(m, "dict", dict)
	if _, ok := m["modeltype"]; !ok {
		add(m, "modeltype", func(t sqltypes.Type) (name string, err error) {
			_, name, err = mc.ModelType(t)
			return
		})
	}
	return t
}
