package secretsource

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"getarcane.dev/arcane/v3/internal/config"
	"getarcane.dev/arcane/v3/internal/container"
	"getarcane.dev/arcane/v3/internal/environment"
	"getarcane.dev/arcane/v3/internal/middleware"
	"getarcane.dev/arcane/v3/internal/project"
	"getarcane.dev/arcane/v3/internal/scheduler/flow"
	"getarcane.dev/arcane/v3/internal/swarm"
	"getarcane.dev/arcane/v3/pkg/agent"
	"getarcane.dev/arcane/v3/pkg/authz"
	"getarcane.dev/arcane/v3/pkg/handlerutil"
)

type (
	Module struct {
		workflow  *flow.Workflow
		handler   *Handler
		workloads *Workloads
	}
	Params struct {
		fx.In
		Service    *Service
		Projects   *project.ProjectService
		Containers *container.ContainerService
		Swarm      *swarm.SwarmService
		Transport  environment.RemoteTransport
		Tunnels    *agent.TunnelRegistry
	}
)

func New(p Params) *Module {
	workloads := &Workloads{service: p.Service, projects: p.Projects, containers: p.Containers, swarm: p.Swarm, transport: p.Transport, tunnels: p.Tunnels}
	return &Module{handler: &Handler{service: p.Service, workloads: workloads}, workloads: workloads}
}

func (m *Module) RegisterRoutes(api huma.API, cfg *config.Config) {
	h := &Handler{}
	if m != nil {
		h = m.handler
	}
	if !cfg.AgentEnabled {
		middleware.RegisterWithPermission(
			api,
			huma.Operation{
				OperationID: "secretSourceTargetKeys",
				Method:      http.MethodPost,
				Path:        "/secret-sources/{sourceId}/keys",
				Tags:        []string{"Secret Sources"},
				Security:    handlerutil.DefaultOperationSecurity(),
			},
			authz.PermSecretSourcesUse,
			h.TargetKeys,
		)
	}
	if cfg.AgentEnabled {
		huma.Register(
			api,
			middleware.RequireSudo(
				api,
				huma.Operation{
					OperationID: "materializeSecretBindings",
					Method:      http.MethodPut,
					Path:        "/agents/{id}/secret-manifest",
					Tags:        []string{"Secret Sources"},
					Security:    handlerutil.DefaultOperationSecurity(),
				},
			),
			h.Manifest,
		)
		huma.Register(
			api,
			middleware.RequireSudo(
				api,
				huma.Operation{
					OperationID: "applyAgentSecrets",
					Method:      http.MethodPost,
					Path:        "/agents/{id}/secret-apply",
					Tags:        []string{"Secret Sources"},
					Security:    handlerutil.DefaultOperationSecurity(),
				},
			),
			h.AgentApply,
		)
		huma.Register(
			api,
			middleware.RequireSudo(
				api,
				huma.Operation{
					OperationID: "cleanupAgentSecretSetup",
					Method:      http.MethodPost,
					Path:        "/agents/{id}/secret-cleanup",
					Tags:        []string{"Secret Sources"},
					Security:    handlerutil.DefaultOperationSecurity(),
				},
			),
			h.AgentCleanup,
		)
		huma.Register(
			api,
			middleware.RequireSudo(
				api,
				huma.Operation{
					OperationID: "agentSecretWorkloadRunning",
					Method:      http.MethodPost,
					Path:        "/agents/{id}/secret-running",
					Tags:        []string{"Secret Sources"},
					Security:    handlerutil.DefaultOperationSecurity(),
				},
			),
			h.AgentRunning,
		)
		return
	}
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "listSecretSources",
			Method:      http.MethodGet,
			Path:        "/secret-sources",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesRead,
		h.ListSources,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "createSecretSource",
			Method:      http.MethodPost,
			Path:        "/secret-sources",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesManage,
		h.CreateSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "updateSecretSource",
			Method:      http.MethodPut,
			Path:        "/secret-sources/{sourceId}",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesManage,
		h.UpdateSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "deleteSecretSource",
			Method:      http.MethodDelete,
			Path:        "/secret-sources/{sourceId}",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesManage,
		h.DeleteSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "testSecretSource",
			Method:      http.MethodPost,
			Path:        "/secret-sources/{sourceId}/test",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesManage,
		h.TestSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "browseSecretSource",
			Method:      http.MethodPost,
			Path:        "/secret-sources/{sourceId}/browse",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.Browse,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "listSecretBindings",
			Method:      http.MethodGet,
			Path:        "/agents/{id}/secret-bindings",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.ListBindings,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "createSecretBinding",
			Method:      http.MethodPost,
			Path:        "/agents/{id}/secret-bindings",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.CreateBinding,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "updateSecretBinding",
			Method:      http.MethodPut,
			Path:        "/agents/{id}/secret-bindings/{bindingId}",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.UpdateBinding,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "deleteSecretBinding",
			Method:      http.MethodDelete,
			Path:        "/agents/{id}/secret-bindings/{bindingId}",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.DeleteBinding,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "checkSecretBindings",
			Method:      http.MethodPost,
			Path:        "/agents/{id}/secret-bindings/check",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.Check,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "applySecretBindings",
			Method:      http.MethodPost,
			Path:        "/agents/{id}/secret-bindings/apply",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSecretSourcesUse,
		h.Apply,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "planInfisicalSetup",
			Method:      http.MethodPost,
			Path:        "/agents/{id}/secret-bindings/setup/plan",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
			Middlewares: middleware.RequirePermission(api, authz.PermSecretSourcesUse),
		},
		authz.PermSecretSourcesManage,
		h.PlanSetup,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "applyInfisicalSetup",
			Method:      http.MethodPost,
			Path:        "/agents/{id}/secret-bindings/setup",
			Tags:        []string{"Secret Sources"},
			Security:    handlerutil.DefaultOperationSecurity(),
			Middlewares: middleware.RequirePermission(api, authz.PermSecretSourcesUse),
		},
		authz.PermSecretSourcesManage,
		h.ApplySetup,
	)
}
