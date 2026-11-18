package secretsource

import (
	"context"
	"encoding/json/v2"
	"errors"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/project"
	"getarcane.dev/arcane/v3/internal/secretsource/children/infisical"
	"getarcane.dev/arcane/v3/pkg/projects"
	"getarcane.dev/types/v3"
)

var composeReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)[^}]*\}`)

func setupVariables(content string, local, remote map[string]string) []types.SecretSetupVariable {
	references := make(map[string]bool)
	for _, match := range composeReference.FindAllStringSubmatch(content, -1) {
		references[match[1]] = true
	}
	keys := make(map[string]bool)
	for key := range local {
		keys[key] = true
	}
	for key := range references {
		keys[key] = true
	}
	variables := make([]types.SecretSetupVariable, 0, len(keys))
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		localValue, hasLocal := local[key]
		remoteValue, hasRemote := remote[key]
		variables = append(
			variables,
			types.SecretSetupVariable{
				Key:               key,
				HasLocalValue:     hasLocal,
				ExistsInInfisical: hasRemote,
				Differs:           hasLocal && hasRemote && localValue != remoteValue,
				Referenced:        references[key],
			},
		)
	}
	return variables
}

func (w *Workloads) PlanSetup(ctx context.Context, environmentID string, req types.SecretSetupRequest) (types.SecretSetupResult, error) {
	content, err := w.ProjectSource(ctx, environmentID, req.Owner)
	if err != nil {
		return types.SecretSetupResult{}, err
	}
	local, err := projects.ParseProjectEnvContent(content.EnvContent, nil)
	if err != nil {
		return types.SecretSetupResult{}, common.Classify(common.ErrValidation, errors.New("unable to parse project variables"))
	}
	source, err := w.service.loadSource(ctx, req.SourceID)
	if err != nil {
		return types.SecretSetupResult{}, err
	}
	target := resolvedTarget(req.Target, source.Settings.Defaults)
	var remote map[string]string
	if target.ProjectID != "" && req.NewProjectName == "" {
		remote, err = w.service.Fetch(ctx, req.SourceID, target)
		if err != nil && !infisical.IsNotFound(err) {
			return types.SecretSetupResult{}, common.Classify(common.ErrUnavailable, errors.New("unable to read Infisical target"))
		}
	}
	return types.SecretSetupResult{Target: target, Variables: setupVariables(content.ComposeContent+"\n"+content.OverrideContent, local, remote)}, nil
}

func (w *Workloads) ApplySetup(ctx context.Context, environmentID string, req types.SecretSetupRequest, actor types.Actor) (types.SecretSetupResult, error) {
	result := types.SecretSetupResult{Target: req.Target}
	if environmentID != "0" {
		if err := w.SyncManifest(ctx, environmentID); err != nil {
			return result, err
		}
	}
	step := func(name string, err error) bool {
		status := "completed"
		if err != nil {
			status = "failed"
		}
		result.Steps = append(result.Steps, types.SecretSetupStep{Name: name, Status: status})
		return err == nil
	}
	content, err := w.ProjectSource(ctx, environmentID, req.Owner)
	if err != nil {
		return result, err
	}
	local, err := projects.ParseProjectEnvContent(content.EnvContent, nil)
	if err != nil {
		return result, common.Classify(common.ErrValidation, errors.New("unable to parse local variables"))
	}
	for _, key := range req.Keys {
		if _, ok := local[key]; !ok || !envKey.MatchString(key) {
			return result, common.Classify(common.ErrValidation, errors.New("select only locally defined variable names"))
		}
	}
	if len(req.Keys) == 0 {
		return result, common.Classify(common.ErrValidation, errors.New("select at least one variable"))
	}
	client, source, err := w.service.client(ctx, req.SourceID, true)
	if err != nil {
		return result, common.Classify(common.ErrValidation, errors.New("configure a separate setup identity first"))
	}
	result.Target = resolvedTarget(req.Target, source.Settings.Defaults)
	if req.NewProjectName != "" {
		existing, listErr := client.ListProjects(ctx)
		if listErr != nil {
			step("create_project", listErr)
			return result, nil
		}
		if slices.ContainsFunc(existing, func(p infisical.Project) bool {
			return strings.EqualFold(p.Name, strings.TrimSpace(req.NewProjectName))
		}) {
			return result, common.Classify(common.ErrConflict, errors.New("an Infisical project with this name already exists"))
		}
		created, createErr := client.CreateProject(ctx, req.NewProjectName)
		if !step("create_project", createErr) {
			return result, nil
		}
		result.Target.ProjectID = created.ID
	}
	if _, err = client.EnsureFolderPath(ctx, result.Target.ProjectID, result.Target.Environment, result.Target.Path); !step("create_folders", err) {
		return result, nil
	}
	remote, err := client.ListSecrets(ctx, infisical.SecretsQuery{ProjectID: result.Target.ProjectID, Environment: result.Target.Environment, SecretPath: result.Target.Path})
	if !step("read_target", err) {
		return result, nil
	}
	var create, update []infisical.SecretInput
	for _, key := range req.Keys {
		value, exists := remote[key]
		if !exists {
			create = append(create, infisical.SecretInput{Key: key, Value: local[key]})
			continue
		}
		if value != local[key] && req.ReplaceExisting {
			update = append(update, infisical.SecretInput{Key: key, Value: local[key]})
		}
	}
	created, err := client.CreateSecrets(ctx, result.Target.ProjectID, result.Target.Environment, result.Target.Path, create)
	if !step("create_secrets", err) {
		return result, nil
	}
	updated, err := client.UpdateSecrets(ctx, result.Target.ProjectID, result.Target.Environment, result.Target.Path, update)
	if !step("update_secrets", err) {
		return result, nil
	}
	if created.PendingApproval || updated.PendingApproval {
		result.Steps = append(result.Steps, types.SecretSetupStep{Name: "approval", Status: "pending"})
		return result, nil
	}
	organizationID, err := client.OrganizationID(ctx)
	if !step("find_organization", err) {
		return result, nil
	}
	identity, err := client.FindIdentityByClientID(ctx, organizationID, source.Settings.ClientID)
	if !step("find_deploy_identity", err) {
		return result, nil
	}
	if identity == nil {
		step("find_deploy_identity", errors.New("deploy identity not found"))
		return result, nil
	}
	if _, err = client.AddProjectIdentity(ctx, result.Target.ProjectID, identity.ID, "viewer"); !step("grant_read_access", err) {
		return result, nil
	}
	values, err := w.service.Fetch(ctx, req.SourceID, result.Target)
	if !step("verify_deploy_identity", err) {
		return result, nil
	}
	for _, key := range req.Keys {
		value, ok := values[key]
		if !ok || value != local[key] {
			step("verify_values", errors.New("value mismatch"))
			return result, nil
		}
	}
	err = w.bindSetupProject(ctx, environmentID, req, result.Target)
	if !step("bind_project", err) {
		return result, nil
	}
	step("sync_agent", nil)
	result.Verified = true
	if req.RemoveLocal {
		step("remove_local_values", w.CleanupSetup(ctx, environmentID, types.SecretCleanupRequest{Owner: req.Owner, Keys: req.Keys, Backup: req.Backup}, actor))
	}
	return result, nil
}

func (w *Workloads) CleanupSetup(ctx context.Context, environmentID string, req types.SecretCleanupRequest, actor types.Actor) error {
	if environmentID != "0" {
		body, err := json.Marshal(req)
		if err != nil {
			return err
		}
		return w.transport.ProxyJSONRequest(ctx, environmentID, http.MethodPost, "/api/agents/0/secret-cleanup", body, nil)
	}
	resolved, err := w.service.ResolveSecrets(ctx, req.Owner)
	if err != nil {
		return err
	}
	expected := make(map[string]string)
	for _, key := range req.Keys {
		if value, exists := resolved.Environment[key]; exists {
			expected[key] = value
		}
	}
	return w.projects.RemoveVerifiedSecrets(ctx, project.SecretCleanupOptions{ProjectID: req.Owner.ID, Expected: expected, Backup: req.Backup, Actor: actor})
}

func (w *Workloads) bindSetupProject(ctx context.Context, environmentID string, req types.SecretSetupRequest, target types.SecretTarget) error {
	mappings := make([]types.SecretMapping, 0, len(req.Keys))
	for _, key := range req.Keys {
		mappings = append(mappings, types.SecretMapping{Key: key, Name: key, Delivery: "environment"})
	}

	existingBindings, listErr := w.service.ListBindings(ctx, environmentID, req.Owner)
	if listErr != nil {
		return listErr
	}
	bindingID := ""
	position := len(existingBindings)
	autoRollout := false
	for _, binding := range existingBindings {
		if binding.SourceID == req.SourceID && binding.Target == target {
			bindingID = binding.ID
			position = binding.Position
			autoRollout = binding.AutoRollout
			break
		}
	}
	_, err := w.SaveBinding(
		ctx,
		environmentID,
		bindingID,
		types.SecretBindingWrite{
			Owner:       req.Owner,
			SourceID:    req.SourceID,
			Target:      target,
			Mappings:    mappings,
			Position:    position,
			AutoRollout: autoRollout,
			Required:    true,
			Enabled:     true,
		},
	)

	return err
}
