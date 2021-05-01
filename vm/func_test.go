package vm_test

import (
	"context"
	"testing"

	"github.com/skillian/expr"
	"github.com/skillian/expr/vm"
	"github.com/skillian/logging"
)

type funcTest struct {
	name   string
	vars   []expr.Var
	values expr.Values
	expr   expr.Expr
	res    interface{}
	errstr string
}

var (
	funcTests = []funcTest{
		func() funcTest {
			vars := []expr.Var{
				&vm.Var{OpType: vm.Int},
				&vm.Var{OpType: vm.Int},
			}
			t := funcTest{
				name: "helloWorld",
				vars: vars,
				values: expr.NewValues(
					expr.VarValuePair{Var: vars[0], Value: 1},
					expr.VarValuePair{Var: vars[1], Value: 2},
				),
				expr: expr.Add{vars[0], vars[1]},
				res:  3,
			}
			return t
		}(),
		{
			name:   "subtraction",
			values: expr.NewValues(),
			expr:   expr.Sub{2, 1},
			res:    1,
		},
		func() funcTest {
			type Person struct {
				Name string
			}
			p := Person{
				Name: "Test Name",
			}
			t := funcTest{
				name:   "getName",
				values: expr.NewValues(),
				expr:   expr.MemOf(&p, &p, &p.Name),
				res:    "Test Name",
			}
			return t
		}(),
		func() funcTest {
			type Third struct{}
			type Second struct {
				*Third
			}
			type First struct {
				*Second
			}
			t := &Third{}
			s := &Second{t}
			f := &First{s}
			return funcTest{
				name:   "multiMember",
				values: expr.NoValues,
				expr:   expr.MemOf(expr.MemOf(f, f, &f.Second), s, &s.Third),
				res:    t,
			}
		}(),
		func() funcTest {
			vars := [...]vm.Var{
				{OpType: vm.Str},
				{OpType: vm.Str},
			}
			vs := expr.NewValues(
				expr.VarValuePair{Var: &vars[0], Value: "abc"},
				expr.VarValuePair{Var: &vars[1], Value: "def"},
			)
			return funcTest{
				name:   "varClosure",
				values: vs,
				expr:   expr.Add{&vars[0], &vars[1]},
				res:    "abcdef",
			}
		}(),
	}

	logger = logging.GetLogger("expr/vm")
)

func TestFunc(t *testing.T) {
	defer logging.TestingHandler(logger, t, logging.HandlerLevel(logging.VerboseLevel))()
	virt, err := vm.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := virt.AddToContext(context.Background())
	for i := range funcTests {
		tc := &funcTests[i]
		t.Run(tc.name, func(t *testing.T) {
			f, err := vm.FuncFromExpr(tc.vars, tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			res, err := f.Call(ctx, tc.values)
			if err != nil {
				t.Fatal(err)
			}
			if res != tc.res {
				t.Fatal("expected:", tc.res, "got:", res)
			}
		})
	}
}
