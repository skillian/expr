package exprlang_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/expr/exprlang"
	"github.com/skillian/logging"
)

var (
	logger = logging.GetLogger(
		exprlang.PkgName,
		logging.LoggerLevel(logging.EverythingLevel),
		logging.LoggerPropagate(true),
	)
)

func TestParse(t *testing.T) {
	defer logging.TestingHandler(logger, t, logging.HandlerLevel(logging.EverythingLevel))()
	for _, tc := range parseTests {
		e, err := exprlang.ParseExpr(
			strings.NewReader(tc.source),
			tc.vs,
		)
		tc.expect.test(t, e, tc.vs, err)
	}
}

var parseTests = []parseTest{
	// {
	// 	source: "1",
	// 	expect: parseTestExpect{
	// 		expr: 1,
	// 	},
	// },
	// {
	// 	source: "1 + 1",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, 1},
	// 	},
	// },
	// {
	// 	source: "1 + 2 * 3",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, 3}},
	// 	},
	// },
	// {
	// 	source: "1+2*3",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, 3}},
	// 	},
	// },
	// {
	// 	source: "1+2*34",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, 34}},
	// 	},
	// },
	// {
	// 	source: "1+2*3.4",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, func() *big.Rat {
	// 			r, _ := (&big.Rat{}).SetString("3.4")
	// 			return r
	// 		}()}},
	// 	},
	// },
	// {
	// 	source: "1+2*3.4.5",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, expr.Mem{
	// 			func() *big.Rat {
	// 				r, _ := (&big.Rat{}).SetString("3.4")
	// 				return r
	// 			}(),
	// 			5,
	// 		}}},
	// 	},
	// },
	// {
	// 	source: "1+2*(3).4",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, expr.Mem{3, 4}}},
	// 	},
	// },
	// {
	// 	source: "1+2*3.4.5.6",
	// 	expect: parseTestExpect{
	// 		expr: expr.Add{1, expr.Mul{2, expr.Mem{
	// 			func() *big.Rat {
	// 				r, _ := (&big.Rat{}).SetString("3.4")
	// 				return r
	// 			}(),
	// 			func() *big.Rat {
	// 				r, _ := (&big.Rat{}).SetString("5.6")
	// 				return r
	// 			}(),
	// 		}}},
	// 	},
	// },
	// {
	// 	source: "1+",
	// 	expect: parseTestExpect{
	// 		errSubstr: "parse terminal: EOF",
	// 	},
	// },
	func() parseTest {
		root := expr.Ident("root")
		return parseTest{
			source: "root.Attribute",
			vs: expr.NewValues(expr.VarValue{
				Var: root,
				Value: &struct {
					Attribute string
				}{
					Attribute: "Hello, World!",
				},
			}),
			expect: parseTestExpect{
				expr: expr.Mem{root, expr.Ident("Attribute")},
			},
		}
	}(),
}

func (pte *parseTestExpect) test(t *testing.T, e expr.Expr, vs expr.Values, err error) {
	t.Helper()
	if e == nil && err == nil {
		t.Fatalf("no error or result")
	}
	if err != nil {
		switch {
		case pte.err != nil:
			if errors.Is(err, pte.err) {
				return
			}
			t.Fatalf(
				"%[1]v (type: %[1]T) is not %[2]v (type: %[2]T)",
				err, pte.err,
			)
		case pte.errSubstr != "":
			errStr := err.Error()
			if !strings.Contains(errStr, pte.errSubstr) {
				t.Fatalf(
					"error %v does not contain %q",
					errStr, pte.errSubstr,
				)
			}
		default:
			t.Fatalf(
				"unexpected error: %v",
				err,
			)
		}
		return
	}
	if pte.expr == nil {
		if pte.err != nil {
			t.Fatalf("expected an error: %v, not: %v", pte.err, e)
		}
		if pte.errSubstr != "" {
			t.Fatalf("expected an error: %q, not: %v", pte.errSubstr, e)
		}
		panic("parse test expect is all nil")
	}
	var eq func(a, b any) bool
	eq = func(a, b any) bool {
		aOps, bOps := expr.Operands(a), expr.Operands(b)
		if len(aOps) != len(bOps) {
			return false
		}
		if len(aOps) > 0 {
			if reflect.TypeOf(a) != reflect.TypeOf(b) {
				return false
			}
			for i, aOp := range aOps {
				if !eq(aOp, bOps[i]) {
					return false
				}
			}
			return true
		}
		f, err := expr.FuncOfExpr(
			ctxutil.Background(),
			expr.Eq{a, b},
			vs,
		)
		if err != nil {
			logger.Error3(
				"creating func for comparing %[1]v (type %[1]T) == %[2]v (type: %[2]T): %[3]v",
				a, b, err,
			)
			return false
		}
		logger.Verbose("%v", f)
		eqObj, err := f.Call(ctxutil.Background(), vs)
		if err != nil {
			logger.Error3(
				"comparing %[1]v (type %[1]T) == %[2]v (type: %[2]T): %[3]v",
				a, b, err,
			)
			return false
		}
		isEq, ok := eqObj.(bool)
		if !ok {
			logger.Error4(
				"comparing %[1]v (type %[1]T) == "+
					"%[2]v (type: %[2]T): "+
					"expected %[3]T, not %[4]v "+
					"(type: %[4]T)",
				a, b, isEq, eqObj,
			)
			return false
		}
		return isEq
	}
	if !eq(e, pte.expr) {
		t.Fatalf(
			"result %[1]v (type %[1]T) != %[2]v (type "+
				"%[2]T)",
			e, pte.expr,
		)
	}
}

type parseTest struct {
	source string
	vs     expr.Values
	expect parseTestExpect
}

type parseTestExpect struct {
	expr      expr.Expr
	err       error
	errSubstr string
}
