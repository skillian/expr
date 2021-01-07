package sqltype_test

import (
	"strings"
	"testing"
	"time"

	"github.com/skillian/expr/stream/sqlstream/sqltype"
)

type parseTest struct {
	str string
	res sqltype.Type
	err string
}

var parseTests = []parseTest{
	{str: "int(32)", res: sqltype.IntType{Bits: 32}, err: ""},
	{str: "string(length: 32)", res: sqltype.StringType{Length: 32}, err: ""},
	{str: "nullable(bool)", res: sqltype.Nullable{sqltype.Bool}, err: ""},
	{str: "nullable(bytes(length: 128, var: true))", res: sqltype.Nullable{sqltype.BytesType{Length: 128, Var: true}}, err: ""},
	{str: "date(min: 1901-01-01, max: 2100-12-31, prec: 1s)", res: sqltype.TimeType{Min: date(1901, 1, 1), Max: date(2100, 12, 31), Prec: 1 * time.Second}, err: ""},
	{str: "date(min: \"January 1, 1800\", max: \"December 20th, 2100\", prec: 1m)", res: sqltype.TimeType{Min: date(1800, 1, 1), Max: date(2100, 12, 20), Prec: 1 * time.Minute}, err: ""},
}

func TestParse(t *testing.T) {
	for _, tc := range parseTests {
		t.Run(tc.str, func(t *testing.T) {
			res, err := sqltype.Parse(tc.str)
			if err != nil {
				if tc.err != "" && strings.Contains(err.Error(), tc.err) {
					return
				}
				t.Fatal(err)
			}
			if res != tc.res {
				t.Fatalf("%q: expected: %v, actual: %v", tc.str, tc.res, res)
			}
		})
	}
}

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}
