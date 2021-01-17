package sqlstream

import (
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/skillian/expr"
	"github.com/skillian/logging"
)

type queryBuilder struct {
	sb strings.Builder
	q  *query

	// qvarnames maps queries to their var names in the SQL
	qvarnames map[*query]string

	// tvarprefixes maps table names to their prefixes
	tvarprefixes map[string]string

	// tvarprefixcounts maps tvarprefixes to a unique number to append to
	// them.  This way joins into tables that have the same var names get
	// unique numbers appended to the end.
	tvarprefixcounts map[string]int

	// numArgs is the total number of args and vars.
	numArgs int

	// sqlArgs holds constant values to pass to (*sql.DB).QueryRows.
	sqlArgs []queryArg

	// sqlVars holds variables resolved from expr.Values and passed to
	// (*sql.DB).QueryRows.
	sqlVars []queryVar
}

var queryBuilderLogger = logging.GetLogger(path.Join(logger.Name(), "queryBuilder"))

func newQueryBuilder(q *query) *queryBuilder {
	qb := &queryBuilder{
		sb:               strings.Builder{},
		q:                q,
		qvarnames:        make(map[*query]string, 8),
		tvarprefixes:     make(map[string]string, 8),
		tvarprefixcounts: make(map[string]int, 8),
	}
	return qb
}

func (qb *queryBuilder) append(ss ...string) {
	for _, s := range ss {
		qb.sb.WriteString(s)
	}
}

func (qb *queryBuilder) sqlifyExpr(e expr.Expr) string {
	type frame struct {
		e      expr.Expr
		opstrs []string
	}
	stack := make([]frame, 1, 8)
	stack[0].opstrs = make([]string, 0, 1)
	expr.Inspect(e, func(e expr.Expr) bool {
		if e == nil {
			queryBuilderLogger.Verbose1("exiting: %v", stack)
			top, sec := stack[len(stack)-1], &stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			var opstr string
			var infix string
			var omitParens bool
			switch e := top.e.(type) {
			case expr.Not:
				opstr = "NOT " + top.opstrs[0]
			case expr.Eq:
				infix = " = "
			case expr.Ne:
				infix = " <> "
			case expr.Gt:
				infix = " > "
			case expr.Ge:
				infix = " >= "
			case expr.Lt:
				infix = " < "
			case expr.Le:
				infix = " <= "
			case expr.Add:
				infix = " + "
			case expr.Sub:
				infix = " - "
			case expr.Mul:
				infix = " * "
			case expr.Div:
				infix = " / "
			case expr.And:
				infix = " AND "
			case expr.Or:
				infix = " OR "
			case expr.Mem:
				omitParens = true
				infix = "."
			case expr.Set:
				omitParens = true
				infix = ", "
			case *query:
				opstr = qb.getQueryVar(e)
				if _, ok := sec.e.(expr.Mem); !ok {
					omitParens = true
					infix = ", "
					for _, c := range e.from.columns {
						top.opstrs = append(top.opstrs, opstr+"."+c.sqlName)
					}
				}
			case expr.Var:
				opstr = "?"
				qb.sqlVars = append(qb.sqlVars, queryVar{e, qb.numArgs})
				qb.numArgs++
			default:
				if m, ok := sec.e.(expr.Mem); ok {
					opstr = qb.q.from.columns[m[1].(int)].sqlName
				} else {
					opstr = "?"
					qb.sqlArgs = append(qb.sqlArgs, queryArg{top.e, qb.numArgs})
					qb.numArgs++
				}
			}
			if infix != "" {
				opstr = strings.Join(top.opstrs, infix)
				if !omitParens {
					opstr = strings.Join(
						[]string{"(", ")"},
						opstr)
				}
			}
			sec.opstrs = append(sec.opstrs, opstr)
			queryBuilderLogger.Verbose1("exited: %v", stack)
			return true
		}
		stack = append(stack, frame{e: e, opstrs: make([]string, 0, 2)})
		queryBuilderLogger.Verbose("entered: %v", stack)
		return true
	})
	return stack[0].opstrs[0]
}

func (qb *queryBuilder) buildSQL() string {
	qb.append("SELECT ")
	if qb.q.limit != 0 && qb.q.db.dialect.DialectFlags().HasAll(DialectTopInfix) {
		qb.append("TOP (")
		qb.append(strconv.FormatInt(qb.q.limit, 10))
		qb.append(") ")
	}
	qv := qb.getQueryVar(qb.q)
	if len(qb.q.joins) == 0 {
		for i, c := range qb.q.from.columns {
			if i > 0 {
				qb.append(", ")
			}
			qb.append(qv, ".")
			qb.append(c.sqlName)
		}
	} else {
		j := qb.q.joins[len(qb.q.joins)-1]
		qb.append(qb.sqlifyExpr(j.then))
	}
	qb.append(" FROM ", qb.q.from.sqlName, " ", qv)
	for _, jq := range qb.q.joins {
		qb.append(" INNER JOIN ", jq.query.from.sqlName, " ", qb.getQueryVar(jq.query), " ON ")
		qb.append(qb.sqlifyExpr(jq.when), " ")
	}
	if qb.q.where != nil {
		qb.append(" WHERE ", qb.sqlifyExpr(qb.q.where))
	}
	// TODO: Sorting
	if qb.q.limit != 0 && qb.q.db.dialect.DialectFlags().HasAll(DialectLimitSuffix) {
		qb.append(" LIMIT ")
		qb.append(strconv.FormatInt(qb.q.limit, 10))
	}
	qb.append(";")
	return qb.sb.String()
}

func (qb *queryBuilder) getQueryVar(q *query) string {
	q = q.base
	varname, ok := qb.qvarnames[q]
	if ok {
		return varname
	}
	prefix, ok := qb.tvarprefixes[q.from.sqlName]
	if !ok {
		short := make([]rune, 0, 8)
		for _, r := range q.from.sqlName {
			if !isAlphanumeric(r) {
				continue
			}
			if len(short) > 0 && unicode.IsLower(r) {
				continue
			}
			short = append(short, unicode.ToLower(r))
		}
		prefix = string(short)
		qb.tvarprefixes[q.from.sqlName] = prefix
	}
	i := qb.tvarprefixcounts[prefix]
	qb.tvarprefixcounts[prefix] = i + 1
	varname = prefix + strconv.Itoa(i)
	qb.qvarnames[q] = varname
	return varname
}
