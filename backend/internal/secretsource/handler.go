package secretsource

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/middleware"
	"getarcane.dev/arcane/v3/pkg/authz"
	"getarcane.dev/arcane/v3/pkg/handlerutil"
	"getarcane.dev/types/v3"
)

type (
	Handler struct {
		service   *Service
		workloads *Workloads
	}

	sourceInput struct {
		ID string `path:"sourceId"`
	}
	sourceWriteInput struct {
		ID   string `path:"sourceId"`
		Body types.SecretSourceWrite
	}
	targetInput struct {
		ID   string `path:"sourceId"`
		Body types.SecretTarget
	}
	bindingsInput struct {
		EnvironmentID string `path:"id"`
		Kind          string `query:"kind" enum:"project,container,stack,service"`
		OwnerID       string `query:"ownerId"`
		ClusterID     string `query:"clusterId"`
	}
	bindingWriteInput struct {
		EnvironmentID string `path:"id"`
		BindingID     string `path:"bindingId"`
		Body          types.SecretBindingWrite
	}
	bindingInput struct {
		EnvironmentID string `path:"id"`
		BindingID     string `path:"bindingId"`
	}
)

func (h *Handler) ListSources(ctx context.Context, _ *struct{}) (*handlerutil.Out[[]types.SecretSource], error) {
	result, err := h.service.ListSources(ctx)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[[]types.SecretSource]{Body: types.ApiResponse[[]types.SecretSource]{Success: true, Data: result}}, nil
}

func (h *Handler) CreateSource(ctx context.Context, input *struct{ Body types.SecretSourceWrite }) (*handlerutil.Out[types.SecretSource], error) {
	result, err := h.service.SaveSource(ctx, "", input.Body)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretSource]{Body: types.ApiResponse[types.SecretSource]{Success: true, Data: result}}, nil
}

func (h *Handler) UpdateSource(ctx context.Context, input *sourceWriteInput) (*handlerutil.Out[types.SecretSource], error) {
	result, err := h.service.SaveSource(ctx, input.ID, input.Body)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretSource]{Body: types.ApiResponse[types.SecretSource]{Success: true, Data: result}}, nil
}

func (h *Handler) DeleteSource(ctx context.Context, input *sourceInput) (*struct{}, error) {
	if err := h.service.DeleteSource(ctx, input.ID); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) TestSource(ctx context.Context, input *targetInput) (*handlerutil.Out[types.SecretCheck], error) {
	result, err := h.service.Test(ctx, input.ID, input.Body)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretCheck]{Body: types.ApiResponse[types.SecretCheck]{Success: true, Data: result}}, nil
}

func (h *Handler) TargetKeys(ctx context.Context, input *targetInput) (*handlerutil.Out[types.SecretCheck], error) {
	values, err := h.service.Fetch(ctx, input.ID, input.Body)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("Infisical target is unavailable; check the target and identity permissions")
	}
	return &handlerutil.Out[types.SecretCheck]{Body: types.ApiResponse[types.SecretCheck]{Success: true, Data: types.SecretCheck{Keys: slices.Sorted(maps.Keys(values))}}}, nil
}

func (h *Handler) Browse(ctx context.Context, input *targetInput) (*handlerutil.Out[types.SecretBrowse], error) {
	result, err := h.service.Browse(ctx, input.ID, input.Body)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("unable to browse Infisical; enter the target manually")
	}
	return &handlerutil.Out[types.SecretBrowse]{Body: types.ApiResponse[types.SecretBrowse]{Success: true, Data: result}}, nil
}

func authorizeOwner(ctx context.Context, environmentID string, owner types.SecretOwner, write bool) error {
	perms, ok := middleware.PermissionsFromContext(ctx)
	if !ok {
		return common.Classify(common.ErrForbidden, errors.New("workload permissions unavailable"))
	}
	permission := authz.PermProjectsRead
	switch owner.Kind {
	case "project":
		if write {
			permission = authz.PermProjectsUpdate
		}
	case "container":
		permission = authz.PermContainersRead
		if write {
			permission = authz.PermContainersUpdate
		}
	case "stack":
		permission = authz.PermSwarmStacksRead
		if write {
			permission = authz.PermSwarmStacksUpdate
		}
	case "service":
		permission = authz.PermSwarmServicesRead
		if write {
			permission = authz.PermSwarmServicesUpdate
		}
	default:
		return common.Classify(common.ErrValidation, errors.New("unsupported workload kind"))
	}
	if !perms.Allows(permission, environmentID) {
		return common.Classify(common.ErrForbidden, errors.New("workload permission denied"))
	}
	return nil
}

func (h *Handler) ListBindings(ctx context.Context, input *bindingsInput) (*handlerutil.Out[[]types.SecretBinding], error) {
	owner := types.SecretOwner{Kind: input.Kind, ID: input.OwnerID, ClusterID: input.ClusterID}
	if err := authorizeOwner(ctx, input.EnvironmentID, owner, false); err != nil {
		return nil, handlerutil.APIError(err)
	}
	owner, err := h.workloads.CanonicalOwner(ctx, input.EnvironmentID, owner)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	result, err := h.service.ListBindings(ctx, input.EnvironmentID, owner)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[[]types.SecretBinding]{Body: types.ApiResponse[[]types.SecretBinding]{Success: true, Data: result}}, nil
}

func (h *Handler) CreateBinding(ctx context.Context, input *struct {
	EnvironmentID string `path:"id"`
	Body          types.SecretBindingWrite
},
) (*handlerutil.Out[types.SecretBinding], error) {
	return h.saveBinding(ctx, input.EnvironmentID, "", input.Body)
}

func (h *Handler) UpdateBinding(ctx context.Context, input *bindingWriteInput) (*handlerutil.Out[types.SecretBinding], error) {
	return h.saveBinding(ctx, input.EnvironmentID, input.BindingID, input.Body)
}

func (h *Handler) saveBinding(ctx context.Context, environmentID, id string, req types.SecretBindingWrite) (*handlerutil.Out[types.SecretBinding], error) {
	if err := authorizeOwner(ctx, environmentID, req.Owner, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	owner, err := h.workloads.CanonicalOwner(ctx, environmentID, req.Owner)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	req.Owner = owner
	if req.AutoRollout {
		perms, ok := middleware.PermissionsFromContext(ctx)
		permission := ""
		if req.Owner.Kind == "project" {
			permission = authz.PermProjectsDeploy
		}
		if req.Owner.Kind == "container" {
			permission = authz.PermContainersRedeploy
		}
		if !ok || permission != "" && !perms.Allows(permission, environmentID) {
			return nil, huma.Error403Forbidden("deployment permission required for automatic rollout")
		}
	}
	result, err := h.workloads.SaveBinding(ctx, environmentID, id, req)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretBinding]{Body: types.ApiResponse[types.SecretBinding]{Success: true, Data: result}}, nil
}

func (h *Handler) DeleteBinding(ctx context.Context, input *bindingInput) (*struct{}, error) {
	binding, err := h.service.GetBinding(ctx, input.EnvironmentID, input.BindingID)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	if err = authorizeOwner(ctx, input.EnvironmentID, binding.Owner, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	if err = h.service.DeleteBinding(ctx, input.EnvironmentID, input.BindingID); err != nil {
		return nil, handlerutil.APIError(err)
	}
	if err = h.workloads.SyncManifest(ctx, input.EnvironmentID); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) Check(ctx context.Context, input *bindingsInput) (*handlerutil.Out[[]types.SecretBinding], error) {
	owner := types.SecretOwner{Kind: input.Kind, ID: input.OwnerID, ClusterID: input.ClusterID}
	if err := authorizeOwner(ctx, input.EnvironmentID, owner, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	owner, canonicalErr := h.workloads.CanonicalOwner(ctx, input.EnvironmentID, owner)
	if canonicalErr != nil {
		return nil, handlerutil.APIError(canonicalErr)
	}
	if _, err := h.service.resolve(ctx, input.EnvironmentID, owner); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return h.ListBindings(ctx, input)
}

func (h *Handler) Manifest(ctx context.Context, input *struct {
	ID   string `path:"id"`
	Body []types.SecretBinding
},
) (*struct{}, error) {
	if err := h.service.Materialize(ctx, input.Body); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) Apply(ctx context.Context, input *struct {
	EnvironmentID string `path:"id"`
	Body          types.SecretOwner
},
) (*struct{}, error) {
	if err := authorizeOwner(ctx, input.EnvironmentID, input.Body, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	perms, ok := middleware.PermissionsFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("workload permissions unavailable")
	}
	permission := ""
	if input.Body.Kind == "project" {
		permission = authz.PermProjectsDeploy
	}
	if input.Body.Kind == "container" {
		permission = authz.PermContainersRedeploy
	}
	if permission != "" && !perms.Allows(permission, input.EnvironmentID) {
		return nil, huma.Error403Forbidden("deployment permission required")
	}
	actor, ok := handlerutil.CurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("authentication required")
	}
	if err := h.workloads.Apply(ctx, input.EnvironmentID, input.Body, *actor); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) AgentApply(ctx context.Context, input *struct {
	ID   string `path:"id"`
	Body types.SecretOwner
},
) (*struct{}, error) {
	if err := h.workloads.Apply(ctx, "0", input.Body, types.SystemUser); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) PlanSetup(ctx context.Context, input *struct {
	EnvironmentID string `path:"id"`
	Body          types.SecretSetupRequest
},
) (*handlerutil.Out[types.SecretSetupResult], error) {
	if err := authorizeOwner(ctx, input.EnvironmentID, input.Body.Owner, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	result, err := h.workloads.PlanSetup(ctx, input.EnvironmentID, input.Body)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretSetupResult]{Body: types.ApiResponse[types.SecretSetupResult]{Success: true, Data: result}}, nil
}

func (h *Handler) ApplySetup(ctx context.Context, input *struct {
	EnvironmentID string `path:"id"`
	Body          types.SecretSetupRequest
},
) (*handlerutil.Out[types.SecretSetupResult], error) {
	if err := authorizeOwner(ctx, input.EnvironmentID, input.Body.Owner, true); err != nil {
		return nil, handlerutil.APIError(err)
	}
	actor, ok := handlerutil.CurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("authentication required")
	}
	result, err := h.workloads.ApplySetup(ctx, input.EnvironmentID, input.Body, *actor)
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[types.SecretSetupResult]{Body: types.ApiResponse[types.SecretSetupResult]{Success: true, Data: result}}, nil
}

func (h *Handler) AgentCleanup(ctx context.Context, input *struct {
	ID   string `path:"id"`
	Body types.SecretCleanupRequest
},
) (*struct{}, error) {
	if err := h.workloads.CleanupSetup(ctx, "0", input.Body, types.SystemUser); err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &struct{}{}, nil
}

func (h *Handler) AgentRunning(ctx context.Context, input *struct {
	ID   string `path:"id"`
	Body types.SecretOwner
},
) (*handlerutil.Out[bool], error) {
	active, err := h.workloads.Running(ctx, rotationTarget{EnvironmentID: "0", Owner: input.Body})
	if err != nil {
		return nil, handlerutil.APIError(err)
	}
	return &handlerutil.Out[bool]{Body: types.ApiResponse[bool]{Success: true, Data: active}}, nil
}
