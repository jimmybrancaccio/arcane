package secretsource

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/types/v3"
)

func (s *Service) Materialize(ctx context.Context, bindings []types.SecretBinding) error {
	if !s.config.AgentEnabled {
		return common.Classify(common.ErrForbidden, errors.New("binding manifests are agent-only"))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&Binding{}).Error; err != nil {
			return err
		}
		for _, b := range bindings {
			if err := validateBinding(b.SecretBindingWrite); err != nil {
				return err
			}
			binding := Binding{
				BaseModel:     database.BaseModel{ID: b.ID},
				EnvironmentID: "0",
				OwnerKind:     b.Owner.Kind,
				OwnerID:       b.Owner.ID,
				ClusterID:     b.Owner.ClusterID,
				SourceID:      b.SourceID,
				Target:        b.Target,
				Mappings:      append([]types.SecretMapping{}, b.Mappings...),
				Position:      b.Position,
				Required:      b.Required,
				Enabled:       b.Enabled,
				AutoRollout:   false,
				Keys:          append([]string{}, b.Keys...),
			}

			if err := tx.Create(&binding).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
