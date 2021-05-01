package stream_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/skillian/logging"

	"github.com/skillian/expr"
	"github.com/skillian/expr/stream"
	"github.com/skillian/expr/vm"
)

var logger = logging.GetLogger("expr", logging.LoggerLevel(logging.VerboseLevel))

type localTest struct {
	name   string
	src    stream.Streamer
	fn     func(s stream.Streamer) (stream.Streamer, error)
	res    []interface{}
	errstr string
}

var localTests = []func() localTest{
	func() localTest {
		return localTest{
			name: "helloWorld",
			src:  &stream.Slice{1, 2, 3, 4, 5},
			fn: func(s stream.Streamer) (stream.Streamer, error) {
				return stream.LineOf(s, func(sl stream.Line) stream.Line {
					return sl.Filter(expr.Lt{sl, 4})
				}, func(sl stream.Line) stream.Line {
					return sl.Map(expr.Mul{sl, 2})
				}), nil
			},
			res: []interface{}{2, 4, 6},
		}
	},
	func() localTest {
		type Docket struct {
			DocketID int
		}
		type Filing struct {
			FilingID int
			DocketID int
			*Docket
		}
		fs := &stream.Slice{
			&Filing{
				FilingID: 100,
				DocketID: 100,
			},
			&Filing{
				FilingID: 101,
				DocketID: 100,
			},
			&Filing{
				FilingID: 102,
				DocketID: 100,
			},
			&Filing{
				FilingID: 110,
				DocketID: 101,
			},
			&Filing{
				FilingID: 111,
				DocketID: 101,
			},
			&Filing{
				FilingID: 112,
				DocketID: 101,
			},
			&Filing{
				FilingID: 120,
				DocketID: 102,
			},
		}
		logger.Verbose1("fs[0] = %p", (*fs)[0])
		ds := &stream.Slice{
			&Docket{DocketID: 102},
			&Docket{DocketID: 100},
			&Docket{DocketID: 101},
		}
		logger.Verbose1("ds[0] = %p", (*ds)[0])
		return localTest{
			name: "join",
			src:  fs,
			fn: func(s stream.Streamer) (stream.Streamer, error) {
				var f Filing
				var d Docket
				return stream.Join(fs, ds, expr.Eq{
					expr.MemOf(fs.Var(), &f, &f.DocketID),
					expr.MemOf(ds.Var(), &d, &d.DocketID),
				}, expr.Set{fs.Var(), ds.Var()})
			},
			res: []interface{}{
				[]interface{}{(*fs)[0], (*ds)[1]},
				[]interface{}{(*fs)[1], (*ds)[1]},
				[]interface{}{(*fs)[2], (*ds)[1]},
				[]interface{}{(*fs)[3], (*ds)[2]},
				[]interface{}{(*fs)[4], (*ds)[2]},
				[]interface{}{(*fs)[5], (*ds)[2]},
				[]interface{}{(*fs)[6], (*ds)[0]},
			},
		}
	},
}

func TestLocal(t *testing.T) {
	defer logging.TestingHandler(logger, t, logging.HandlerLevel(logging.VerboseLevel))()
	virt, err := vm.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := virt.AddToContext(context.Background())
	for i := range localTests {
		tc := localTests[i]()
		t.Run(tc.name, func(t *testing.T) {
			vs := expr.NewValues()
			ctx = expr.AddValuesToContext(ctx, vs)
			s, err := tc.fn(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			v := s.Var()
			res := make([]interface{}, 0, len(tc.res))
			if err = stream.Each(ctx, s, func(s stream.Stream) error {
				x := vs.Get(v)
				//xs := x.([]interface{})
				t.Logf("got: %#v", x)
				res = append(res, x)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(tc.res) != len(res) {
				t.Fatal("expected", len(tc.res), "results, but got", len(res))
			}
			for i, r := range res {
				if !reflect.DeepEqual(tc.res[i], r) {
					t.Fatal("expected", tc.res[i], "at index", i, "but got", r)
				}
			}
		})
	}
}
