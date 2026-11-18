package secretsource

import (
	"context"
	"log/slog"

	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/arcane/v3/internal/event"
	"getarcane.dev/arcane/v3/pkg/handlerutil"
	"getarcane.dev/types/v3"
)

func (s *Service) audit(ctx context.Context, kind event.EventType, binding Binding) {
	actor := types.SystemUser
	if user, ok := handlerutil.CurrentUserFromContext(ctx); ok {
		actor = *user
	}
	if err := s.events.LogResourceEvent(ctx, event.ResourceEvent{
		Type: kind, ResourceType: binding.OwnerKind, ResourceID: binding.OwnerID, ResourceName: binding.OwnerID,
		UserID: actor.ID, Username: actor.Username, EnvironmentID: binding.EnvironmentID,
		Metadata: database.JSON{"sourceId": binding.SourceID, "bindingId": binding.ID},
	}); err != nil {
		slog.WarnContext(ctx, "failed to record Infisical audit event", "bindingId", binding.ID, "error", err)
	}
}
