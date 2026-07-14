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

type Company struct {
	ID        string `orm:"pk"`
	Name      string
	CreatedAt time.Time `orm:"created_at"`
	UpdatedAt time.Time `orm:"updated_at"`
}

type User struct {
	ID        string `orm:"pk"`
	CompanyID string `orm:"company"`
	Email     string `orm:"unique"`
	Name      string
	CreatedAt time.Time `orm:"created_at"`
	UpdatedAt time.Time `orm:"updated_at"`
}

type Product struct {
	ID          string `orm:"pk"`
	CompanyID   string `orm:"company"`
	WorkspaceID string `orm:"workspace"`
	Name        string
	DeletedAt   *time.Time `orm:"soft_delete"`
	CreatedAt   time.Time  `orm:"created_at"`
	UpdatedAt   time.Time  `orm:"updated_at"`
}

func main() {
	ctx := context.Background()
	runID := time.Now().UTC().Format("20060102150405")
	db := openDB(ctx)
	defer db.Close()

	seed(ctx, db, runID)
	showPolicies(ctx, db, runID)
}

func seed(ctx context.Context, db *dorm.DB, runID string) {
	now := time.Now().UTC()
	companyA := "company-a-" + runID
	companyB := "company-b-" + runID

	mustCreate(db, ctxFor("system", "", ""), Company{
		ID:        companyA,
		Name:      "Acme",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreate(db, ctxFor("system", "", ""), Company{
		ID:        companyB,
		Name:      "Globex",
		CreatedAt: now,
		UpdatedAt: now,
	})

	mustCreate(db, ctxFor("alice", companyA, "workspace-a"), User{
		ID:        "user-a-" + runID,
		Email:     "alice+" + runID + "@acme.example",
		Name:      "Alice",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreate(db, ctxFor("bob", companyB, "workspace-b"), User{
		ID:        "user-b-" + runID,
		Email:     "bob+" + runID + "@globex.example",
		Name:      "Bob",
		CreatedAt: now,
		UpdatedAt: now,
	})

	mustCreate(db, ctxFor("alice", companyA, "workspace-a"), Product{
		ID:        "product-a1-" + runID,
		Name:      "Notebook",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreate(db, ctxFor("alice", companyA, "workspace-a"), Product{
		ID:        "product-a2-" + runID,
		Name:      "Pen",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreate(db, ctxFor("bob", companyB, "workspace-a"), Product{
		ID:        "product-b1-" + runID,
		Name:      "Keyboard",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreate(db, ctxFor("bob", companyB, "workspace-b"), Product{
		ID:        "product-b2-" + runID,
		Name:      "Mouse",
		CreatedAt: now,
		UpdatedAt: now,
	})

	pen := Product{ID: "product-a2-" + runID}
	if err := db.WithContext(ctxFor("alice", companyA, "workspace-a")).Delete(&pen); err != nil {
		fatal(err)
	}
}

func showPolicies(ctx context.Context, db *dorm.DB, runID string) {
	scope := access.WithContext(ctx, access.Context{
		UserID:      "viewer",
		CompanyID:   "company-a-" + runID,
		WorkspaceID: "workspace-a",
	})

	printProducts("default policy", db.WithContext(scope))
	printProducts("ignore company", db.WithPolicy(access.IgnoreCompany()).WithContext(scope))
	printProducts("ignore rls", db.WithPolicy(access.IgnoreRLS()).WithContext(scope))
	printProducts("system mode", db.WithPolicy(access.System()).WithContext(scope))
}

func printProducts(label string, session *orm.Session) {
	var products []Product
	if err := session.Find(&products, orm.OrderBy("name ASC")); err != nil {
		fatal(err)
	}
	fmt.Printf("%s: ", label)
	for i, product := range products {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(product.Name)
	}
	if len(products) == 0 {
		fmt.Print("(none)")
	}
	fmt.Println()
}

func mustCreate(db *dorm.DB, ctx context.Context, model any) {
	if err := db.WithContext(ctx).Create(model); err != nil {
		fatal(err)
	}
}

func ctxFor(userID, companyID, workspaceID string) context.Context {
	return access.WithContext(context.Background(), access.Context{
		UserID:      userID,
		CompanyID:   companyID,
		WorkspaceID: workspaceID,
	})
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
