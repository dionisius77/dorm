package model

import "time"

// Company marks a model as company-scoped.
type Company struct {
	CompanyID string
}

// Audit marks a model with lifecycle metadata.
type Audit struct {
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt *time.Time
	DeletedBy string
}

// Version marks a model with optimistic-locking metadata.
type Version struct {
	Version int64
}

// Entity is the common managed model trait.
type Entity struct {
	Company
	Audit
}
