package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/access"
	driverpg "github.com/dionisius77/dorm/driver/postgres"
	"github.com/dionisius77/dorm/orm"
)

type User struct {
	ID        string `orm:"pk"`
	Email     string `orm:"unique"`
	Name      string
	CreatedAt time.Time `orm:"created_at"`
	UpdatedAt time.Time `orm:"updated_at"`
}

func main() {
	ctx := access.WithContext(context.Background(), access.Context{
		UserID: "demo-user",
	})
	runID := time.Now().UTC().Format("20060102150405")

	db := openDB(ctx)
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		fatal(err)
	}

	session := db.WithContext(ctx)
	now := time.Now().UTC()
	user := User{
		ID:        "user-" + runID,
		Email:     "alice+" + runID + "@example.com",
		Name:      "Alice",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := session.Create(&user); err != nil {
		fatal(err)
	}

	var users []User
	if err := session.Find(&users, orm.Where("email = ?", user.Email)); err != nil {
		fatal(err)
	}
	fmt.Printf("created %d user(s)\n", len(users))

	user.Name = "Alice Updated"
	if err := session.Update(&user); err != nil {
		fatal(err)
	}

	if err := session.Delete(&user); err != nil {
		fatal(err)
	}
}

func openDB(ctx context.Context) *dorm.DB {
	dsn := requiredDSN()
	db, err := dorm.Open(ctx, driverpg.New(driverpg.Config{DSN: dsn}))
	if err != nil {
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
