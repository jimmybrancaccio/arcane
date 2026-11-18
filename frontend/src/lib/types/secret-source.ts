export interface SecretTarget {
	projectId: string;
	environment: string;
	path: string;
}

export interface SecretSourceSettings {
	host: string;
	clientId: string;
	setupClientId: string;
	defaults: SecretTarget;
}

export interface SecretSource {
	id: string;
	name: string;
	settings: SecretSourceSettings;
	default: boolean;
	hasCredential: boolean;
	hasSetupCredential: boolean;
	lastTestedAt?: string;
	lastError?: string;
}

export interface SecretSourceWrite {
	name: string;
	settings: SecretSourceSettings;
	credential: string;
	setupCredential: string;
	default: boolean;
}

export interface SecretOwner {
	kind: 'project' | 'container' | 'stack' | 'service';
	id: string;
	clusterId?: string;
}

export interface SecretMapping {
	key: string;
	name: string;
	delivery: 'environment' | 'file';
	service?: string;
	uid?: string;
	gid?: string;
	mode?: number;
}

export interface SecretBindingWrite {
	owner: SecretOwner;
	sourceId: string;
	target: SecretTarget;
	mappings: SecretMapping[];
	position: number;
	required: boolean;
	enabled: boolean;
	autoRollout: boolean;
}

export interface SecretBinding extends SecretBindingWrite {
	id: string;
	environmentId: string;
	deployedAt?: string;
	checkedAt?: string;
	changed: boolean;
	keys: string[];
	lastError?: string;
}

export interface SecretCheck {
	keys: string[];
	changed: boolean;
	warning?: string;
}

export interface SecretSetupRequest {
	owner: SecretOwner;
	sourceId: string;
	target: SecretTarget;
	keys: string[];
	newProjectName: string;
	replaceExisting: boolean;
	removeLocal: boolean;
	backup: boolean;
}

export interface SecretSetupResult {
	target: SecretTarget;
	variables: { key: string; hasLocalValue: boolean; existsInInfisical: boolean; differs: boolean; referenced: boolean }[];
	steps: { name: string; status: string }[];
	verified: boolean;
}

export interface SecretBrowseProject {
	id: string;
	name: string;
	environments: { name: string; slug: string }[];
}
export interface SecretBrowse {
	projects: SecretBrowseProject[];
	folders: string[];
}
