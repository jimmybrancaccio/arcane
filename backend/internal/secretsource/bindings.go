package secretsource

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/event"
	"getarcane.dev/arcane/v3/pkg/agent"
	"getarcane.dev/types/v3"
)

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func bindingDTO(b Binding) types.SecretBinding {
	return types.SecretBinding{
		ID: b.ID, EnvironmentID: b.EnvironmentID,
		SecretBindingWrite: types.SecretBindingWrite{
			Owner:    types.SecretOwner{Kind: b.OwnerKind, ID: b.OwnerID, ClusterID: b.ClusterID},
			SourceID: b.SourceID, Target: b.Target, Mappings: b.Mappings, Position: b.Position,
			Required: b.Required, Enabled: b.Enabled, AutoRollout: b.AutoRollout,
		},
		DeployedAt: b.DeployedAt, CheckedAt: b.CheckedAt, Changed: b.DeployedHash != "" && b.SeenHash != b.DeployedHash,
		Keys: b.Keys, LastError: b.LastError,
	}
}

func (s *Service) ownerQuery(ctx context.Context, environmentID string, owner types.SecretOwner) *gorm.DB {
	q := s.db.WithContext(ctx).Where("owner_kind = ? AND owner_id = ?", owner.Kind, owner.ID)
	if owner.ClusterID != "" {
		return q.Where("cluster_id = ?", owner.ClusterID)
	}
	return q.Where("environment_id = ?", environmentID)
}

func (s *Service) ListBindings(ctx context.Context, environmentID string, owner types.SecretOwner) ([]types.SecretBinding, error) {
	var bindings []Binding
	q := s.db.WithContext(ctx).Order("position, created_at, id")
	if owner.Kind != "" {
		q = s.ownerQuery(ctx, environmentID, owner).Order("position, created_at, id")
	} else {
		q = q.Where("environment_id = ?", environmentID)
	}
	if err := q.Find(&bindings).Error; err != nil {
		return nil, err
	}
	result := make([]types.SecretBinding, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, bindingDTO(binding))
	}
	return result, nil
}

func validateBinding(req types.SecretBindingWrite) error {
	if !slices.Contains([]string{"project", "container", "stack", "service"}, req.Owner.Kind) || strings.TrimSpace(req.Owner.ID) == "" {
		return errors.New("a supported workload kind and ID are required")
	}
	if (req.Owner.Kind == "stack" || req.Owner.Kind == "service") && req.Owner.ClusterID == "" {
		return errors.New("a Swarm cluster ID is required")
	}
	seen := make(map[string]bool)
	for _, m := range req.Mappings {
		if m.Key == "" || m.Name == "" {
			return errors.New("each mapping needs a secret key and destination")
		}
		switch m.Delivery {
		case "environment":
			if !envKey.MatchString(m.Name) {
				return errors.New("invalid environment variable name")
			}
		case "file":
			if req.Owner.Kind != "stack" && req.Owner.Kind != "service" {
				return errors.New("native secret files require a Swarm workload")
			}
			if path.IsAbs(m.Name) || path.Clean(m.Name) != m.Name || strings.Contains(m.Name, "/") || m.Name == "." || m.Name == ".." {
				return errors.New("secret file target must be a filename")
			}
			for _, identity := range []string{m.UID, m.GID} {
				if identity != "" {
					if _, err := strconv.ParseUint(identity, 10, 32); err != nil {
						return errors.New("secret file UID and GID must be numeric")
					}
				}
			}
			if m.Mode > 0o777 {
				return errors.New("invalid secret file mode")
			}
		default:
			return errors.New("delivery must be environment or file")
		}
		key := m.Service + "/" + m.Delivery + "/" + m.Name
		if seen[key] {
			return errors.New("duplicate secret destination")
		}
		seen[key] = true
	}
	return nil
}

func (s *Service) SaveBinding(ctx context.Context, environmentID, id string, req types.SecretBindingWrite) (types.SecretBinding, error) {
	if err := validateBinding(req); err != nil {
		return types.SecretBinding{}, common.Classify(common.ErrValidation, err)
	}
	source, err := s.loadSource(ctx, req.SourceID)
	if err != nil {
		return types.SecretBinding{}, err
	}
	target := resolvedTarget(req.Target, source.Settings.Defaults)
	if target.ProjectID == "" || target.Environment == "" {
		return types.SecretBinding{}, common.Classify(common.ErrValidation, errors.New("an Infisical project and environment are required"))
	}
	var previous Binding
	if id != "" {
		if loadErr := s.db.WithContext(ctx).First(&previous, "id = ? AND environment_id = ?", id, environmentID).Error; loadErr != nil {
			return types.SecretBinding{}, loadErr
		}
		if previous.OwnerKind != req.Owner.Kind || previous.OwnerID != req.Owner.ID || previous.ClusterID != req.Owner.ClusterID {
			return types.SecretBinding{}, common.Classify(common.ErrValidation, errors.New("a binding's owner cannot be changed"))
		}
	}
	if previous.Salt == "" {
		previous.Salt = rand.Text()
	}
	// GORM serializes nil slices as SQL NULL instead of JSON arrays.
	binding := Binding{
		BaseModel:     previous.BaseModel,
		EnvironmentID: environmentID,
		OwnerKind:     req.Owner.Kind,
		OwnerID:       req.Owner.ID,
		ClusterID:     req.Owner.ClusterID,
		SourceID:      req.SourceID,
		Target:        target,
		Mappings:      append([]types.SecretMapping{}, req.Mappings...),
		Position:      req.Position,
		Required:      req.Required,
		Enabled:       req.Enabled,
		AutoRollout:   req.AutoRollout,
		Salt:          previous.Salt,
		DeployedHash:  previous.DeployedHash,
		DeployedAt:    previous.DeployedAt,
		Keys:          append([]string{}, previous.Keys...),
	}

	if saveErr := s.db.WithContext(ctx).Save(&binding).Error; saveErr != nil {
		return types.SecretBinding{}, saveErr
	}
	return bindingDTO(binding), nil
}

func (s *Service) DeleteBinding(ctx context.Context, environmentID, id string) error {
	return s.db.WithContext(ctx).Where("environment_id = ?", environmentID).Delete(&Binding{}, "id = ?", id).Error
}

func hashValues(salt string, values map[string]string) string {
	h := hmac.New(sha256.New, []byte(salt))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		encoded, _ := json.Marshal([]string{key, values[key]})
		_, _ = h.Write(encoded)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mappedValues(binding Binding, values map[string]string) (types.SecretResolution, error) {
	result := types.SecretResolution{Environment: make(map[string]string), ServiceEnvironment: make(map[string]map[string]string)}
	if len(binding.Mappings) == 0 {
		for key, value := range values {
			if envKey.MatchString(key) {
				result.Environment[key] = value
			}
		}
		return result, nil
	}
	for _, mapping := range binding.Mappings {
		value, ok := values[mapping.Key]
		if !ok {
			if binding.Required {
				return types.SecretResolution{}, fmt.Errorf("required key %q is missing", mapping.Key)
			}
			continue
		}
		if mapping.Delivery == "environment" {
			if mapping.Service == "" {
				result.Environment[mapping.Name] = value
			} else {
				if result.ServiceEnvironment[mapping.Service] == nil {
					result.ServiceEnvironment[mapping.Service] = make(map[string]string)
				}
				result.ServiceEnvironment[mapping.Service][mapping.Name] = value
			}
			continue
		}
		result.Files = append(
			result.Files,
			types.SecretFileValue{
				Version: hashValues(binding.Salt, map[string]string{mapping.Key: value}),
				Key:     mapping.Key,
				Name:    mapping.Name,
				Service: mapping.Service,
				UID:     cmp.Or(mapping.UID, "0"),
				GID:     cmp.Or(mapping.GID, "0"),
				Mode:    cmp.Or(mapping.Mode, 0o400),
				Value:   value,
			},
		)
	}
	return result, nil
}

func (s *Service) resolve(ctx context.Context, environmentID string, owner types.SecretOwner) (types.SecretResolution, error) {
	var bindings []Binding
	if err := s.ownerQuery(ctx, environmentID, owner).Where("enabled = ?", true).Order("position, created_at, id").Find(&bindings).Error; err != nil {
		return types.SecretResolution{}, err
	}
	result := types.SecretResolution{Owner: owner, Environment: make(map[string]string), ServiceEnvironment: make(map[string]map[string]string), Hashes: make(map[string]string)}
	files := make(map[string]bool)
	for _, binding := range bindings {
		values, fetchErr := s.Fetch(ctx, binding.SourceID, binding.Target)
		resolved, mapErr := mappedValues(binding, values)
		fetchErr = errors.Join(fetchErr, mapErr)
		if fetchErr == nil && binding.Required && len(resolved.Environment)+len(resolved.ServiceEnvironment)+len(resolved.Files) == 0 {
			fetchErr = errors.New("required binding returned no usable secrets")
		}
		status := map[string]any{"checked_at": time.Now(), "last_error": ""}
		if fetchErr != nil {
			status["last_error"] = "Infisical fetch failed; check the source, target and required keys"
			s.audit(ctx, event.EventTypeSecretFetchError, binding)
			if err := s.db.WithContext(ctx).Model(&Binding{}).Where("id = ?", binding.ID).Updates(status).Error; err != nil {
				return types.SecretResolution{}, err
			}
			if binding.Required {
				return types.SecretResolution{}, common.Classify(common.ErrUnavailable, errors.New("required Infisical binding is unavailable"))
			}
			continue
		}
		hash := hashResolution(binding.Salt, resolved)
		s.audit(ctx, event.EventTypeSecretFetch, binding)
		if binding.DeployedHash != "" && hash != binding.DeployedHash && hash != binding.SeenHash {
			s.audit(ctx, event.EventTypeSecretChanged, binding)
		}
		status["seen_hash"] = hash
		status["keys"] = slices.Sorted(maps.Keys(values))
		// Use the serializer for key names; map Updates bypasses it.
		keysJSON, _ := json.Marshal(slices.Sorted(maps.Keys(values)))
		status["keys"] = string(keysJSON)
		if err := s.db.WithContext(ctx).Model(&Binding{}).Where("id = ?", binding.ID).Updates(status).Error; err != nil {
			return types.SecretResolution{}, err
		}
		result.Hashes[binding.ID] = hash
		earlierGlobal := maps.Clone(result.Environment)
		for key, value := range resolved.Environment {
			if _, exists := result.Environment[key]; !exists {
				result.Environment[key] = value
			}
		}
		for service, values := range resolved.ServiceEnvironment {
			if result.ServiceEnvironment[service] == nil {
				result.ServiceEnvironment[service] = make(map[string]string)
			}
			for key, value := range values {
				if _, earlier := earlierGlobal[key]; earlier {
					continue
				}
				if _, exists := result.ServiceEnvironment[service][key]; !exists {
					result.ServiceEnvironment[service][key] = value
				}
			}
		}
		for _, file := range resolved.Files {
			key := file.Service + "/" + file.Name
			if !files[key] {
				result.Files = append(result.Files, file)
				files[key] = true
			}
		}
	}
	return result, nil
}

func (s *Service) ResolveSecrets(ctx context.Context, owner types.SecretOwner) (types.SecretResolution, error) {
	if s.config.AgentEnabled {
		return agent.RequestSecrets(ctx, types.SecretTunnelRequest{Owner: owner})
	}

	return s.resolve(ctx, "0", owner)
}

func (s *Service) record(ctx context.Context, environmentID string, owner types.SecretOwner, hashes map[string]string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for id, hash := range hashes {
			q := tx.Model(&Binding{}).Where("id = ? AND owner_kind = ? AND owner_id = ?", id, owner.Kind, owner.ID)
			if owner.ClusterID != "" {
				q = q.Where("cluster_id = ?", owner.ClusterID)
			} else {
				q = q.Where("environment_id = ?", environmentID)
			}
			updated := q.Where("seen_hash = ?", hash).Updates(map[string]any{"deployed_hash": hash, "deployed_at": time.Now()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return common.Classify(common.ErrConflict, errors.New("binding changed during deployment; check secrets before retrying"))
			}
		}
		return nil
	})
}

func (s *Service) RecordSecretDeployment(ctx context.Context, owner types.SecretOwner, result types.SecretResolution) error {
	if len(result.Hashes) == 0 {
		return nil
	}
	if s.config.AgentEnabled {
		_, err := agent.RequestSecrets(ctx, types.SecretTunnelRequest{Owner: owner, Hashes: result.Hashes, Acknowledge: true})
		return err
	}
	if result.Owner.Kind != "" {
		owner = result.Owner
	}
	return s.record(ctx, "0", owner, result.Hashes)
}

func (s *Service) AgentRequest(ctx context.Context, environmentID string, req types.SecretTunnelRequest) (types.SecretResolution, error) {
	// Ignore a claimed cluster scope: only this authenticated environment's bindings are available.
	var bindings []Binding
	if err := s.db.WithContext(ctx).Where("environment_id = ? AND owner_kind = ? AND owner_id = ?", environmentID, req.Owner.Kind, req.Owner.ID).Find(&bindings).Error; err != nil {
		return types.SecretResolution{}, err
	}
	if len(bindings) == 0 {
		return types.SecretResolution{}, nil
	}
	owner := types.SecretOwner{Kind: req.Owner.Kind, ID: req.Owner.ID}
	if req.Acknowledge {
		return types.SecretResolution{}, s.record(ctx, environmentID, owner, req.Hashes)
	}
	return s.resolve(ctx, environmentID, owner)
}

func hashResolution(salt string, resolved types.SecretResolution) string {
	effective := make(map[string]string)
	for key, value := range resolved.Environment {
		effective["environment/"+key] = value
	}
	for service, environment := range resolved.ServiceEnvironment {
		for key, value := range environment {
			effective["environment/"+service+"/"+key] = value
		}
	}
	for _, file := range resolved.Files {
		effective[fmt.Sprintf("file/%s/%s/%s/%s/%o", file.Service, file.Name, file.UID, file.GID, file.Mode)] = file.Value
	}
	return hashValues(salt, effective)
}

func (s *Service) SecretVariableNames(ctx context.Context, owner types.SecretOwner) ([]string, error) {
	var bindings []Binding
	if err := s.ownerQuery(ctx, "0", owner).Where("enabled = ?", true).Find(&bindings).Error; err != nil {
		return nil, err
	}
	keys := make(map[string]bool)
	for _, binding := range bindings {
		if len(binding.Mappings) == 0 {
			for _, key := range binding.Keys {
				if envKey.MatchString(key) {
					keys[key] = true
				}
			}
			continue
		}
		for _, mapping := range binding.Mappings {
			if mapping.Delivery == "environment" {
				keys[mapping.Name] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(keys)), nil
}

func (s *Service) GetBinding(ctx context.Context, environmentID, id string) (types.SecretBinding, error) {
	var binding Binding
	if err := s.db.WithContext(ctx).First(&binding, "id = ? AND environment_id = ?", id, environmentID).Error; err != nil {
		return types.SecretBinding{}, err
	}
	return bindingDTO(binding), nil
}
