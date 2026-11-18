package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"getarcane.dev/types/v3"
)

func TestVerifiedSecretCleanupPreservesReferencesAndCreatesPrivateBackup(t *testing.T) {
	service, proj, _ := newProjectServiceForOverrideTest(t, "secret-cleanup")
	content := "# Keep comments\nPASSWORD=matching-value\nOTHER=keep-me\nTOKEN=referenced-value\nURL=${TOKEN}\n"
	require.NoError(t, os.WriteFile(filepath.Join(proj.Path, ".env"), []byte(content), 0o600))
	require.NoError(
		t,
		service.RemoveVerifiedSecrets(
			t.Context(),
			SecretCleanupOptions{
				ProjectID: proj.ID,
				Expected:  map[string]string{"PASSWORD": "matching-value", "TOKEN": "referenced-value", "OTHER": "changed-value"},
				Actor:     types.SystemUser,
				Backup:    true,
			},
		),
	)
	updated, err := os.ReadFile(filepath.Join(proj.Path, ".env"))
	require.NoError(t, err)
	require.NotContains(t, string(updated), "PASSWORD=")
	require.Contains(t, string(updated), "# Keep comments")
	require.Contains(t, string(updated), "OTHER=keep-me")
	require.Contains(t, string(updated), "TOKEN=referenced-value")
	backups, err := filepath.Glob(filepath.Join(proj.Path, ".env.infisical-*.bak"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	info, err := os.Stat(backups[0])
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCreateProjectKeepsExternalSecretValuesOutOfSavedFiles(t *testing.T) {
	service, _, _ := newProjectServiceForOverrideTest(t, "existing-project")
	compose := "services:\n  app:\n    image: nginx:alpine\n    environment:\n      PASSWORD: ${PASSWORD:?required}\n"
	created, err := service.CreateProject(t.Context(), CreateProjectOptions{Name: "external-secrets", ComposeContent: compose, SecretKeys: []string{"PASSWORD"}, Actor: types.SystemUser})
	require.NoError(t, err)
	saved, err := os.ReadFile(filepath.Join(created.Path, "compose.yaml"))
	require.NoError(t, err)
	require.Equal(t, compose, string(saved))
	_, err = os.Stat(filepath.Join(created.Path, ".env"))
	require.True(t, os.IsNotExist(err))
}
