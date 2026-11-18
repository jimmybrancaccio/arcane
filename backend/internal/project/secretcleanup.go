package project

import (
	"context"
	"errors"
	"strings"
	"time"

	"kit.getarcane.app/acfs"

	"getarcane.dev/arcane/v3/pkg/projects"
	"getarcane.dev/types/v3"
)

type (
	SecretCleanupOptions struct {
		Expected  map[string]string
		ProjectID string
		Actor     types.Actor
		Backup    bool
	}
)

func (s *ProjectService) RemoveVerifiedSecrets(ctx context.Context, opts SecretCleanupOptions) error {
	unlock, err := s.LockProject(ctx, "remove verified Infisical variables", opts.ProjectID)
	if err != nil {
		return err
	}
	defer unlock()
	proj, err := s.getMutableProject(ctx, opts.ProjectID)
	if err != nil {
		return err
	}
	state, err := projects.ReadProjectEnvState(ctx, proj.Path)
	if err != nil {
		return err
	}
	git, err := projects.ParseProjectEnvContent(state.GitContent, nil)
	if err != nil {
		return err
	}
	before, err := projects.ParseProjectEnvContent(state.EditableContent, nil)
	if err != nil {
		return err
	}
	var kept []string
	removed := make(map[string]bool)
	for line := range strings.SplitSeq(state.EditableContent, "\n") {
		key, rawValue, found := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export ")), "=")
		value, selected := opts.Expected[key]
		_, gitOwned := git[key]
		if !found || !selected || gitOwned || before[key] != value ||
			strings.ContainsAny(rawValue, "'\"\\") || strings.Contains(state.EditableContent, "${"+key) || strings.Contains(state.EditableContent, "$"+key) {
			kept = append(kept, line)
			continue
		}
		removed[key] = true
	}
	content := strings.Join(kept, "\n")
	after, err := projects.ParseProjectEnvContent(content, nil)
	if err != nil {
		return err
	}
	for key, value := range before {
		if !removed[key] && after[key] != value {
			return errors.New("cleanup would change another variable; local values were retained")
		}
	}
	if len(removed) == 0 {
		return nil
	}
	if opts.Backup {
		backupPath := "/.env.infisical-" + time.Now().UTC().Format("20060102T150405.000000000") + ".bak"
		if backupErr := (acfs.Root{Path: proj.Path}).Write(ctx, backupPath, []byte(state.EditableContent), acfs.WriteOptions{Mode: 0o600}); backupErr != nil {
			return backupErr
		}
	}
	_, err = s.updateProject(ctx, opts.ProjectID, nil, nil, &content, nil, opts.Actor)
	return err
}
