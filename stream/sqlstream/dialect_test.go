package sqlstream_test

import (
	"testing"

	"github.com/skillian/expr/stream/sqlstream"
)

type namerApplyTest struct {
	source      string
	camelCase   string
	defaultCase string
	pascalCase  string
	snakeCase   string
}

var namerApplyTests = []namerApplyTest{
	{source: "hello world", camelCase: "helloWorld", pascalCase: "HelloWorld", defaultCase: "HelloWorld", snakeCase: "hello_world"},
	{source: "this is a test", camelCase: "thisIsATest", pascalCase: "ThisIsATest", defaultCase: "ThisIsATest", snakeCase: "this_is_a_test"},
	{source: "is   sql", camelCase: "isSql", pascalCase: "IsSql", defaultCase: "IsSQL", snakeCase: "is_sql"},
	{source: "unique id", camelCase: "uniqueId", pascalCase: "UniqueId", defaultCase: "UniqueID", snakeCase: "unique_id"},
	{source: "todo id", camelCase: "todoId", defaultCase: "TodoID", pascalCase: "TodoId", snakeCase: "todo_id"},
}

func TestNamerApplyTest(t *testing.T) {
	for _, tc := range namerApplyTests {
		camelCase := sqlstream.CamelCase.Apply(tc.source)
		if camelCase != tc.camelCase {
			t.Errorf("camelCase: %q should have become %q, not %q", tc.source, tc.camelCase, camelCase)
		}
		defaultCase := sqlstream.DefaultCase.Apply(tc.source)
		if defaultCase != tc.defaultCase {
			t.Errorf("DefaultCase: %q should have become %q, not %q", tc.source, tc.defaultCase, defaultCase)
		}
		pascalCase := sqlstream.PascalCase.Apply(tc.source)
		if pascalCase != tc.pascalCase {
			t.Errorf("PascalCase: %q should have become %q, not %q", tc.source, tc.pascalCase, pascalCase)
		}
		snakeCase := sqlstream.SnakeCase.Apply(tc.source)
		if snakeCase != tc.snakeCase {
			t.Errorf("snake_case: %q should have become %q, not %q", tc.source, tc.snakeCase, snakeCase)
		}
	}
}

type namerParseTest struct {
	camelCase   string
	defaultCase string
	pascalCase  string
	snakeCase   string
	target      string
}

var namerParseTests = []namerParseTest{
	{camelCase: "helloWorld", defaultCase: "HelloWorld", pascalCase: "HelloWorld", snakeCase: "hello_world", target: "hello world"},
	{camelCase: "thisIsATest", defaultCase: "ThisIsATest", pascalCase: "ThisIsATest", snakeCase: "this_is_a_test", target: "this is a test"},
	{camelCase: "isSqlData", defaultCase: "IsSQLData", pascalCase: "isSqlData", snakeCase: "is_sql_data", target: "is sql data"},
	{camelCase: "todoId", defaultCase: "TodoID", pascalCase: "TodoId", snakeCase: "todo_id", target: "todo id"},
}

func TestNamerParseTest(t *testing.T) {
	for _, tc := range namerParseTests {
		target := sqlstream.CamelCase.Parse(tc.camelCase)
		if target != tc.target {
			t.Errorf("camelCase: %q should have parsed to %q, not %q", tc.camelCase, tc.target, target)
		}
		target = sqlstream.DefaultCase.Parse(tc.defaultCase)
		if target != tc.target {
			t.Errorf("DefaultCase: %q should have become %q, not %q", tc.defaultCase, tc.target, target)
		}
		target = sqlstream.PascalCase.Parse(tc.pascalCase)
		if target != tc.target {
			t.Errorf("PascalCase: %q should have become %q, not %q", tc.pascalCase, tc.target, target)
		}
		target = sqlstream.SnakeCase.Parse(tc.snakeCase)
		if target != tc.target {
			t.Errorf("snake_case: %q should have become %q, not %q", tc.snakeCase, tc.target, target)
		}
	}
}
