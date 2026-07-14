package testutil

import (
	"time"
)

const AppModelsSource = `package models

import "time"

type Company struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}

type User struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	CompanyID string    ` + "`orm:\"company\"`" + `
	Email     string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}

type Product struct {
	ID          string     ` + "`orm:\"pk\"`" + `
	CompanyID   string     ` + "`orm:\"company\"`" + `
	WorkspaceID string     ` + "`orm:\"workspace\"`" + `
	SKU         string     ` + "`orm:\"unique\"`" + `
	Name        string
	Description string
	DeletedAt   *time.Time ` + "`orm:\"soft_delete\"`" + `
	CreatedAt   time.Time  ` + "`orm:\"created_at\"`" + `
	UpdatedAt   time.Time  ` + "`orm:\"updated_at\"`" + `
}

type AuditRecord struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	CompanyID string    ` + "`orm:\"company\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	CreatedBy string    ` + "`orm:\"created_by\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
	UpdatedBy string    ` + "`orm:\"updated_by\"`" + `
}

type Role struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Code      string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}
`

const MigrationModelsSourceV2 = `package models

import "time"

type Company struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}

type User struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	CompanyID string    ` + "`orm:\"company\"`" + `
	Email     string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}

type Product struct {
	ID          string     ` + "`orm:\"pk\"`" + `
	CompanyID   string     ` + "`orm:\"company\"`" + `
	WorkspaceID string     ` + "`orm:\"workspace\"`" + `
	SKU         string     ` + "`orm:\"unique\"`" + `
	Name        string
	Description string
	Category    string
	DeletedAt   *time.Time ` + "`orm:\"soft_delete\"`" + `
	CreatedAt   time.Time  ` + "`orm:\"created_at\"`" + `
	UpdatedAt   time.Time  ` + "`orm:\"updated_at\"`" + `
}

type AuditRecord struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	CompanyID string    ` + "`orm:\"company\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	CreatedBy string    ` + "`orm:\"created_by\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
	UpdatedBy string    ` + "`orm:\"updated_by\"`" + `
}

type Role struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Code      string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}
`

const SeedModelsSource = `package models

import "time"

type Role struct {
	ID        string    ` + "`orm:\"pk\"`" + `
	Code      string    ` + "`orm:\"unique\"`" + `
	Name      string
	CreatedAt time.Time ` + "`orm:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`orm:\"updated_at\"`" + `
}
`

var _ = time.Time{}

type Company struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type User struct {
	ID        string
	CompanyID string
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Product struct {
	ID          string
	CompanyID   string
	WorkspaceID string
	SKU         string
	Name        string
	Description string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AuditRecord struct {
	ID        string
	CompanyID string
	Name      string
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
}

type Role struct {
	ID        string
	Code      string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
