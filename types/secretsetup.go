package types

type (
	SecretSetupRequest struct {
		Owner           SecretOwner  `json:"owner"`
		SourceID        string       `json:"sourceId"`
		Target          SecretTarget `json:"target"`
		NewProjectName  string       `json:"newProjectName,omitempty"`
		Keys            []string     `json:"keys"`
		ReplaceExisting bool         `json:"replaceExisting"`
		RemoveLocal     bool         `json:"removeLocal"`
		Backup          bool         `json:"backup"`
	}
	SecretSetupVariable struct {
		Key               string `json:"key"`
		HasLocalValue     bool   `json:"hasLocalValue"`
		ExistsInInfisical bool   `json:"existsInInfisical"`
		Differs           bool   `json:"differs"`
		Referenced        bool   `json:"referenced"`
	}
	SecretSetupStep struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	SecretSetupResult struct {
		Target    SecretTarget          `json:"target"`
		Variables []SecretSetupVariable `json:"variables"`
		Steps     []SecretSetupStep     `json:"steps"`
		Verified  bool                  `json:"verified"`
	}
	SecretCleanupRequest struct {
		Owner  SecretOwner `json:"owner"`
		Keys   []string    `json:"keys"`
		Backup bool        `json:"backup"`
	}
)
