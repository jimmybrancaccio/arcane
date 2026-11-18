package project

import (
	"context"
	"encoding/json/v2"
	"io"
	"maps"
	"slices"

	composetypes "github.com/compose-spec/compose-go/v2/types"

	"kit.getarcane.app/docker/types"

	"getarcane.dev/arcane/v3/pkg/projects"
	"getarcane.dev/arcane/v3/pkg/redact"
	shared "getarcane.dev/types/v3"
)

type (
	secretExecutionKey struct{}
)

func projectSecretEnvironment(ctx context.Context) projects.EnvMap {
	result, _ := ctx.Value(secretExecutionKey{}).(shared.SecretResolution)
	return result.Environment
}

func applyProjectSecrets(ctx context.Context, model *composetypes.Project) {
	resolved, ok := ctx.Value(secretExecutionKey{}).(shared.SecretResolution)
	if !ok || len(resolved.Hashes) == 0 {
		return
	}
	for name, service := range model.Services {
		values := make(map[string]string)
		maps.Copy(values, resolved.Environment)
		maps.Copy(values, resolved.ServiceEnvironment[name])
		env := make(composetypes.MappingWithEquals)
		maps.Copy(env, service.Environment)
		for key, value := range values {
			env[key] = new(value)
		}
		labels := make(composetypes.Labels)
		maps.Copy(labels, service.Labels)
		encoded, _ := json.Marshal(slices.Sorted(maps.Keys(values)))
		labels["getarcane.app/secret-keys"] = string(encoded)
		service.Environment = env
		service.Labels = labels
		model.Services[name] = service
	}
}

func (s *ProjectService) beginSecretDeployment(ctx context.Context, projectID string) (context.Context, func(error) error, error) {
	owner := shared.SecretOwner{Kind: "project", ID: projectID}
	result, err := s.secrets.ResolveSecrets(ctx, owner)
	if err != nil {
		return ctx, nil, err
	}
	ctx = context.WithValue(ctx, secretExecutionKey{}, result)
	values := make(map[string]string)
	maps.Copy(values, result.Environment)
	for service, environment := range result.ServiceEnvironment {
		for key, value := range environment {
			values[service+"/"+key] = value
		}
	}
	mask := redact.NewRedactor(values)
	var writer io.WriteCloser
	if progress, ok := ctx.Value(types.ProgressWriterKey{}).(io.Writer); ok {
		writer = mask.Writer(progress)
		ctx = context.WithValue(ctx, types.ProgressWriterKey{}, io.Writer(writer))
	}
	return ctx, func(deployErr error) error {
		if writer != nil {
			_ = writer.Close()
		}
		if deployErr != nil {
			return mask.Error(deployErr)
		}
		return s.secrets.RecordSecretDeployment(ctx, owner, result)
	}, nil
}

func externalSecretValidation(keys []string) projects.EnvMap {
	values := make(projects.EnvMap, len(keys))
	// Validate structure without writing credentials or synthetic values to project files.
	for _, key := range keys {
		values[key] = "1"
	}
	return values
}
