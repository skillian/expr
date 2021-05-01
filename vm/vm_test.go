package vm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/skillian/expr"
	"github.com/skillian/expr/vm"
)

type vmTest struct {
	name   string
	vmfn   *vm.Func
	args   expr.Values
	res    interface{}
	errstr string
}

var vmTests = [...]vmTest{
	{
		name: "helloWorld",
		vmfn: vm.MustNewFunc(
			nil, vm.OpCodes{vm.OpCode(vm.Nop)}, nil),
		args: expr.NoValues,
	},
	func() vmTest {
		t := vmTest{
			name: "onePlusOne",
			vmfn: vm.MustNewFunc(
				nil, vm.OpCodes{
					vm.EncodeOp(vm.LdConst, vm.Int, 0),
					vm.EncodeOp(vm.LdConst, vm.Int, 0),
					vm.EncodeOp(vm.Add, vm.Int, vm.OpArg(vm.Int)),
					vm.EncodeOp(vm.StStack, vm.Int, 0),
				}, []vm.Var{{OpType: vm.Int}}[:1:1], vm.FuncConsts(vm.MakeValues(1))),
			args: expr.NoValues,
			res:  int(2),
		}
		return t
	}(),
	func() vmTest {
		params := []vm.Var{
			{OpType: vm.Str},
			{OpType: vm.Str},
			{OpType: vm.Str},
		}
		t := vmTest{
			name: "argPlusArg",
			vmfn: vm.MustNewFunc(
				params[1:], vm.OpCodes{
					//vm.EncodeOp(vm.LdStack, vm.Str, 1),
					//vm.EncodeOp(vm.LdStack, vm.Str, 2),
					vm.EncodeOp(vm.Add, vm.Str, vm.OpArg(vm.Str)),
					vm.EncodeOp(vm.StStack, vm.Str, 0),
				}, params[:1], vm.FuncConsts(vm.MakeValues())),
			args: expr.NewValues(
				expr.VarValuePair{Var: &params[1], Value: "hello "},
				expr.VarValuePair{Var: &params[2], Value: "world!"},
			),
			res: "hello world!",
		}
		return t
	}(),
	{
		name: "stackOrderGood",
		vmfn: vm.MustNewFunc(nil, vm.OpCodes{
			vm.EncodeOp(vm.LdConst, vm.Str, 0),
			vm.EncodeOp(vm.LdConst, vm.Str, 1),
			vm.EncodeOp(vm.Add, vm.Str, 0),
		}, []vm.Var{{OpType: vm.Str}}, vm.FuncConsts(
			vm.MakeValues("hello", "world"),
		)),
		args: expr.NoValues,
		res:  "helloworld",
	},
}

func TestVM(t *testing.T) {
	virt, err := vm.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := virt.AddToContext(context.Background())
	for i := range vmTests {
		tc := &vmTests[i]
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.vmfn.Call(ctx, tc.args)
			if err != nil {
				if tc.errstr != "" {
					errstr := err.Error()
					if !strings.Contains(errstr, tc.errstr) {
						t.Fatal(err)
					}
				}
				t.Fatal(err)
			}
			switch x := res.(type) {
			case []interface{}:
				expects, ok := tc.res.([]interface{})
				if !ok {
					t.Fatalf(
						"expected %[1]T, not %[2]v (type: %[2]T)",
						tc.res, x)
				}
				if len(x) != len(expects) {
					t.Fatalf(
						"expected %[1]d results, not %[2]d (%[3]v (type: %[3]T))",
						len(expects), len(x), x)
				}
				for i, expect := range expects {
					if x[i] != expect {
						t.Fatalf(
							"result %d expected %v, got %v",
							i, expect, x[i])
					}
				}
			default:
				if x != tc.res {
					t.Fatalf("expected %v, got %v", tc.res, x)
				}
			}
		})
	}
}
