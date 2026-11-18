package secretsource

import (
	"context"
	"errors"
	"time"

	"github.com/italypaleale/francis/builtin/workflow"

	"getarcane.dev/arcane/v3/internal/scheduler/flow"
	"getarcane.dev/types/v3"
)

type (
	rotationTarget struct {
		EnvironmentID string            `json:"environmentId"`
		Owner         types.SecretOwner `json:"owner"`
	}
)

func (s *Service) DriftTargets(ctx context.Context) ([]rotationTarget, error) {
	var bindings []Binding
	if err := s.db.WithContext(ctx).Where("enabled = ? AND deployed_hash <> ?", true, "").Order("environment_id, position, id").Find(&bindings).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var targets []rotationTarget
	for _, binding := range bindings {
		owner := bindingDTO(binding).Owner
		key := binding.EnvironmentID + "/" + owner.Kind + "/" + owner.ID
		if owner.ClusterID != "" {
			key = owner.ClusterID + "/" + owner.Kind + "/" + owner.ID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := s.resolve(ctx, binding.EnvironmentID, owner); err != nil {
			continue
		}
		current, err := s.ListBindings(ctx, binding.EnvironmentID, owner)
		if err != nil {
			return nil, err
		}
		for _, item := range current {
			if item.Enabled && item.AutoRollout && item.Changed && item.LastError == "" {
				targets = append(targets, rotationTarget{EnvironmentID: binding.EnvironmentID, Owner: owner})
				break
			}
		}
	}
	return targets, nil
}

func (w *Workloads) Rotate(ctx context.Context, target rotationTarget) error {
	resolved, err := w.service.resolve(ctx, target.EnvironmentID, target.Owner)
	if err != nil {
		return err
	}
	var bindings []Binding
	if listErr := w.service.ownerQuery(ctx, target.EnvironmentID, target.Owner).Find(&bindings).Error; listErr != nil {
		return listErr
	}
	changed := false
	for _, binding := range bindings {
		hash, ok := resolved.Hashes[binding.ID]
		if ok && binding.Enabled && binding.AutoRollout && binding.DeployedHash != "" && hash != binding.DeployedHash && hash != binding.AttemptedHash {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	active, err := w.Running(ctx, target)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	for id, hash := range resolved.Hashes {
		if attemptErr := w.service.db.WithContext(ctx).Model(&Binding{}).Where("id = ?", id).Update("attempted_hash", hash).Error; attemptErr != nil {
			return attemptErr
		}
	}
	if err = w.Apply(ctx, target.EnvironmentID, target.Owner, types.SystemUser); err != nil {
		_ = w.service.ownerQuery(ctx, target.EnvironmentID, target.Owner).Model(&Binding{}).Update("last_error", "automatic secret rollout failed; Apply to retry").Error
		return errors.New("automatic secret rollout failed; Apply to retry")
	}
	return nil
}

func (m *Module) RegisterWorkflows(engine *flow.Engine) error {
	wf, err := engine.Define(flow.Definition{
		Name: "secret-drift", Version: 1, Fingerprint: "e900a36c2efb4a56490517d8377ce22c6a69df894a71c5413aab7b64b8c60259", Concurrency: 1, Timeout: 30 * time.Minute,
		Steps: []workflow.StepSpec{
			workflow.Step("discover", engine.Handler(func(ctx context.Context, _ flow.Task) (any, error) { return m.workloads.service.DriftTargets(ctx) })),
			workflow.ForEach("rotate", engine.Handler(func(ctx context.Context, task flow.Task) (any, error) {
				var target rotationTarget
				if err := task.DecodeItem(&target); err != nil {
					return nil, err
				}
				return nil, m.workloads.Rotate(ctx, target)
			}), workflow.WithItemsFrom("discover"), workflow.WithMaxParallel(1), workflow.WithMaxAttempts(1), workflow.WithFailurePolicy(workflow.TolerateFailures)),
			workflow.Step("finalize", engine.Handler(func(ctx context.Context, task flow.Task) (any, error) {
				results, err := flow.Results[struct{}](task, "rotate")
				if err != nil {
					return nil, err
				}
				status := types.RunStatusSucceeded
				for _, result := range results {
					if result.Err != "" {
						status = types.RunStatusPartial
					}
				}
				return types.SchedulerOutcome{Status: status}, nil
			})),
		},
	})
	if err != nil {
		return err
	}
	m.workflow = wf
	return nil
}

func (m *Module) DriftJob(engine *flow.Engine) *flow.Job {
	return &flow.Job{
		Engine:      engine,
		Workflow:    m.workflow,
		JobName:     "secret-drift",
		ScheduleFn:  func(context.Context) string { return "*/5 * * * *" },
		ShouldRunFn: func(context.Context) bool { return !m.workloads.service.config.AgentEnabled },
	}
}
