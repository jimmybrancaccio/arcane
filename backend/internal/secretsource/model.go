package secretsource

import (
	"time"

	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/types/v3"
)

type (
	Source struct {
		LastTestedAt *time.Time
		Settings     types.SecretSourceSettings `gorm:"serializer:json"`
		database.BaseModel
		Name            string
		Credential      string
		SetupCredential string
		LastError       string
		IsDefault       bool
	}
	Binding struct {
		CheckedAt  *time.Time
		DeployedAt *time.Time
		Target     types.SecretTarget `gorm:"serializer:json"`
		database.BaseModel
		AttemptedHash string
		ClusterID     string
		SourceID      string
		LastError     string
		EnvironmentID string `gorm:"index"`
		OwnerKind     string
		Salt          string
		DeployedHash  string
		SeenHash      string
		OwnerID       string
		Keys          []string              `gorm:"serializer:json"`
		Mappings      []types.SecretMapping `gorm:"serializer:json"`
		Position      int
		AutoRollout   bool
		Enabled       bool
		Required      bool
	}
)

func (Source) TableName() string  { return "secret_sources" }
func (Binding) TableName() string { return "secret_bindings" }
