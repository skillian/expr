package sqlstream_test

import (
	"context"
	sql "database/sql"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream"
	"github.com/skillian/expr/stream/sqlstream"
	"github.com/skillian/logging"

	_ "github.com/mattn/go-sqlite3"
)

var logger = logging.GetLogger("expr/stream", logging.LoggerLevel(logging.VerboseLevel))

type queryTest struct {
	name string
	test func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error
}

var queryTests = []queryTest{
	{
		name: "helloWorld",
		test: func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error {
			t := Todo{TodoName: "test #1"}
			m, err := sqlstream.ModelOf(&t)
			if err != nil {
				return errors.Errorf1From(
					err, "failed to create model from "+
						"%[1]v (type: %[1]T)", m)
			}
			db.Save(ctx, m)
			if t.TodoID == 0 {
				return errors.Errorf0(
					"expected TodoID to be set")
			}
			todos, err := db.Query(ctx, m)
			if err != nil {
				return errors.Errorf1From(
					err, "error while querying for "+
						"model %v", m)
			}
			todos, err = stream.Filter(todos, expr.Eq{
				expr.MemOf(todos.Var(), &t, &t.TodoID),
				int64(t.TodoID),
			})
			if err != nil {
				return errors.Errorf1From(
					err, "error attempting to filter %v",
					todos)
			}
			vs.Set(todos.Var(), m)
			streamedTodos := make([]Todo, 0, 8)
			if err = stream.Each(ctx, todos, func(s stream.Stream) error {
				logger.Verbose("got %v", spew.Sdump(t))
				streamedTodos = append(streamedTodos, t)
				return nil
			}); err != nil {
				return errors.Errorf1From(
					err, "error attempting to stream %v",
					todos)
			}
			if len(streamedTodos) != 1 {
				return errors.Errorf2(
					"expected %d, not %d",
					1, len(streamedTodos))
			}
			if streamedTodos[0].TodoName != "test #1" {
				return errors.Errorf2(
					"expected %q, not %q",
					"test #1", t.TodoName)
			}
			return nil
		},
	},
	{
		name: "join2",
		test: func(ctx context.Context, vs expr.Values, db *sqlstream.DB) error {
			todos := [...]*Todo{
				{TodoName: "test 101"},
				{TodoName: "test 102"},
			}
			err := db.Save(ctx, todos[0], todos[1])
			if err != nil {
				return errors.Errorf0From(
					err, "failed to save 2 Todos")
			}
			todoDeps := [...]*TodoDep{
				{TodoID: todos[0].TodoID, DepID: todos[1].TodoID},
			}
			err = db.Save(ctx, todoDeps[0])
			if err != nil {
				return errors.Errorf0From(
					err, "failed to save a TodoDep")
			}
			todo, todoDep := &Todo{}, &TodoDep{}
			todoModel, todoDepModel, err := sqlstream.ModelsOf2(todo, todoDep)
			if err != nil {
				return errors.Errorf0From(err, "failed to create models")
			}
			todoQ, err := db.Query(ctx, todoModel)
			if err != nil {
				return errors.Errorf0From(
					err, "failed to create todo query")
			}
			todoDepQ, err := db.Query(ctx, todoDepModel)
			if err != nil {
				return errors.Errorf0From(
					err, "failed to create joining query")
			}
			s, err := stream.Join(todoDepQ, todoQ, expr.Eq{
				expr.MemOf(todoDepQ.Var(), todoDep, &todoDep.DepID),
				expr.MemOf(todoQ.Var(), todo, &todo.TodoID),
			}, todoQ.Var())
			if err != nil {
				return errors.Errorf2From(
					err, "failed to join %v to %v",
					todoDeps, todos)
			}
			//logger.Verbose0(spew.Sdump(s))
			vs.Set(s.Var(), todo)
			var joined []Todo
			if err = stream.Each(ctx, s, func(s stream.Stream) error {
				joined = append(joined, *todo)
				return nil
			}); err != nil {
				return errors.Errorf0From(
					err, "failed to execute join stream")
			}
			if len(joined) != 1 {
				return errors.Errorf1("expected 1, got %d", len(joined))
			}
			if joined[0].TodoID != 2 {
				return errors.Errorf("expected todo ID 102, not %d", joined[0].TodoID)
			}
			return nil
		},
	},
}

func TestQuery(t *testing.T) {
	defer logging.TestingHandler(logger, t,
		logging.HandlerLevel(logging.VerboseLevel))()
	vs := expr.NewValues()
	ctx := expr.AddValuesToContext(context.Background(), vs)
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

func (t *Todo) ID() sqlstream.Model {
	return sqlstream.ModelField(t, (*int64)(&t.TodoID))
}

func (t *Todo) AppendValues(vs []interface{}) []interface{} {
	return append(vs, int64(t.TodoID), t.TodoName)
}

type TodoDep struct {
	TodoID TodoID
	DepID  TodoID
}

func (t *TodoDep) ID() sqlstream.Model {
	return sqlstream.Compound(
		sqlstream.ModelField(t, (*int64)(&t.TodoID)),
		sqlstream.ModelField(t, (*int64)(&t.DepID)))
}

func (t *TodoDep) AppendValues(vs []interface{}) []interface{} {
	return append(vs, int64(t.TodoID), int64(t.DepID))
}
