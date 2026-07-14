package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dionisius77/dorm"
	driverpg "github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/orm"
)

type Todo struct {
	ID          string `orm:"pk"`
	Title       string
	Description string
	Completed   bool
	CreatedAt   time.Time `orm:"created_at"`
	UpdatedAt   time.Time `orm:"updated_at"`
}

func main() {
	ctx := context.Background()
	runID := time.Now().UTC().Format("20060102150405")
	db := openDB(ctx)
	defer db.Close()

	todo := createTodo(ctx, db, Todo{
		ID:          "todo-" + runID,
		Title:       "Write documentation",
		Description: "Turn the ORM example into a real README",
	})

	listTodos(ctx, db)
	todo = findTodoByID(ctx, db, todo.ID)
	todo = updateTodo(ctx, db, todo, "Write better documentation")
	todo = markComplete(ctx, db, todo)
	deleteTodo(ctx, db, todo.ID)
}

func createTodo(ctx context.Context, db *dorm.DB, todo Todo) Todo {
	if err := db.WithContext(ctx).Create(&todo); err != nil {
		fatal(err)
	}
	fmt.Println("created todo:", todo.ID)
	return todo
}

func listTodos(ctx context.Context, db *dorm.DB) {
	var todos []Todo
	if err := db.WithContext(ctx).Find(&todos, orm.OrderBy("created_at DESC")); err != nil {
		fatal(err)
	}
	fmt.Printf("todo count: %d\n", len(todos))
}

func findTodoByID(ctx context.Context, db *dorm.DB, id string) Todo {
	var todos []Todo
	if err := db.WithContext(ctx).Find(&todos, orm.Where("id = ?", id), orm.Limit(1)); err != nil {
		fatal(err)
	}
	if len(todos) > 0 {
		fmt.Println("found todo:", todos[0].Title)
		return todos[0]
	}
	return Todo{ID: id}
}

func updateTodo(ctx context.Context, db *dorm.DB, todo Todo, title string) Todo {
	todo.Title = title
	if err := db.WithContext(ctx).Update(&todo); err != nil {
		fatal(err)
	}
	fmt.Println("updated todo:", todo.ID)
	return todo
}

func markComplete(ctx context.Context, db *dorm.DB, todo Todo) Todo {
	todo.Completed = true
	if err := db.WithContext(ctx).Update(&todo); err != nil {
		fatal(err)
	}
	fmt.Println("marked complete:", todo.ID)
	return todo
}

func deleteTodo(ctx context.Context, db *dorm.DB, id string) {
	todo := Todo{ID: id}
	if err := db.WithContext(ctx).Delete(&todo); err != nil {
		fatal(err)
	}
	fmt.Println("deleted todo:", todo.ID)
}

func openDB(ctx context.Context) *dorm.DB {
	dsn := requiredDSN()
	db, err := dorm.Open(ctx, driverpg.New(driverpg.Config{DSN: dsn}))
	if err != nil {
		fatal(err)
	}
	if err := db.Ping(ctx); err != nil {
		fatal(err)
	}
	return db
}

func requiredDSN() string {
	for _, key := range []string{"DORM_EXAMPLE_DSN", "DATABASE_URL"} {
		if dsn := os.Getenv(key); dsn != "" {
			return dsn
		}
	}
	fatal(fmt.Errorf("set DORM_EXAMPLE_DSN or DATABASE_URL"))
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
