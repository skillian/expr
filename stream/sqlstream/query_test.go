package sqlstream_test

import (
	"context"
	sql "database/sql"
	"fmt"
	"testing"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream"
	"github.com/skillian/expr/stream/sqlstream"
	"github.com/skillian/logging"

	_ "github.com/mattn/go-sqlite3"
)

type queryTest struct {
	name string
	test func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error
}

var queryTests = []queryTest{
	{
		name: "helloWorld",
		test: func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error {
			t := Todo{TodoID: 100, TodoName: "test #100"}
			m, err := sqlstream.ModelOf(&t)
			if err != nil {
				return errors.Errorf1From(
					err, "failed to create model from "+
						"%[1]v (type: %[1]T)", m)
			}
			db.Save(ctx, m)
			todos, err := db.Query(ctx, m)
			if err != nil {
				return errors.Errorf1From(
					err, "error while querying for "+
						"model %v", m)
			}
			todos, err = stream.Filter(todos, expr.Eq{
				expr.MemOf(todos.Var(), &t, &t.TodoID),
				101,
			})
			if err != nil {
				return errors.Errorf1From(
					err, "error attempting to filter %v",
					todos)
			}
			vs.Set(todos.Var(), m)
			if err = stream.Each(ctx, todos, func(s stream.Stream) error {
				if t.TodoName != "testing 101" {
					return errors.Errorf2("expected %q, not %q", "testing 101", t.TodoName)
				}
				return nil
			}); err != nil {
				return errors.Errorf1From(
					err, "error attempting to stream %v",
					todos)
			}
			return nil
		},
	},
	{
		name: "join",
		test: func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error {
			err := db.Save(ctx,
				&Todo{TodoID: 101, TodoName: "test 101"},
				&Todo{TodoID: 102, TodoName: "test 102"},
				&TodoDep{TodoID: 101, DepID: 102})
			if err != nil {
				return errors.Errorf0From(
					err, "failed to save 2 Todos and a TodoDep")
			}
			todo, todoDep, dep := &Todo{}, &TodoDep{}, &Todo{}
			todoModel, todoDepModel, depModel := sqlstream.MustModelOf(todo), sqlstream.MustModelOf(todoDep), sqlstream.MustModelOf(dep)
			todos, err := db.Query(ctx, todoModel)
			if err != nil {
				return errors.Errorf0From(
					err, "failed to create todo query")
			}
			deps, err := db.Query(ctx, depModel)
			if err != nil {
				return errors.Errorf0From(
					err, "failed to create dependency query")
			}
			todoDeps, err := db.Query(ctx, todoDepModel)
			if err != nil {
				return errors.Errorf0From(
					err, "failed to create joining query")
			}
			joinStream, joinVar := stream.LineOf(todoDeps).Join(
				todos, expr.Eq{
					expr.MemOf(todoDeps.Var(), todoDep, &todoDep.TodoID),
					expr.MemOf(todos.Var(), todo, &todo.TodoID),
				}, todos.Var())
			joinStream, joinVar = joinStream.Join(
				deps, expr.Eq{
					expr.MemOf(joinVar, todoDep, &todoDep.DepID),
					expr.MemOf(deps.Var(), dep, &dep.TodoID),
				}, expr.Set{todos.Var(), deps.Var()})
			vs.Set(todos.Var(), todo)
			vs.Set(deps.Var(), dep)
			var joined [][2]Todo
			if err = stream.Each(ctx, joinStream, func(s stream.Stream) error {
				joined = append(joined, [2]Todo{*todo, *dep})
				return nil
			}); err != nil {
				return errors.Errorf0From(
					err, "failed to execute join stream")
			}
			if len(joined) != 1 {
				return errors.Errorf1("expected 1, got %d", len(joined))
			}
			if joined[0][0].TodoID != 101 {
				return errors.Errorf1("expected left to be todoID 101, not %d", joined[0][0].TodoID)
			}
			if joined[0][1].TodoID != 102 {
				return errors.Errorf1("expected right to be todoID 102, not %d", joined[0][1].TodoID)
			}
			return nil
		},
	},
}

func TestQuery(t *testing.T) {
	defer logging.TestingHandler(
		logging.GetLogger(
			"expr/stream",
			logging.LoggerLevel(logging.VerboseLevel)), t,
		logging.HandlerLevel(logging.VerboseLevel))()
	vs := expr.NewValues()
	ctx, _ := expr.AddValuesToContext(context.Background(), vs)
	for i := range queryTests {
		i := i
		tc := &queryTests[i]
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, err := sql.Open("sqlite3", fmt.Sprintf("file:memdb%d?cache=private&mode=memory", i))
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := sqlstream.NewDB(
				sqlDB,
				sqlstream.WithDialect(sqlstream.SQLite3),
				sqlstream.WithTableNamer(sqlstream.SnakeCase),
				sqlstream.WithColumnNamer(sqlstream.SnakeCase))
			if err != nil {
				t.Fatal(err)
			}
			if err = db.CreateCollection(ctx, &Todo{}); err != nil {
				t.Fatal(err)
			}
			if err = db.CreateCollection(ctx, &TodoDep{}); err != nil {
				t.Fatal(err)
			}
			if err := tc.test(ctx, vs, db); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type TodoID int64

type Todo struct {
	TodoID
	TodoName string
}

func (t *Todo) AppendValues(vs []interface{}) []interface{} {
	return append(vs, int64(t.TodoID), t.TodoName)
}

type TodoDep struct {
	TodoID TodoID
	DepID  TodoID
}

func (t *TodoDep) AppendValues(vs []interface{}) []interface{} {
	return append(vs, int64(t.TodoID), int64(t.DepID))
}
