import { parseDocument } from 'yaml';

import type { SecretMapping } from '../types/secret-source.js';

export interface SecretMappingSuggestions {
	mappings: SecretMapping[];
	services: string[];
	unmatchedKeys: string[];
	status: 'available' | 'missing' | 'invalid';
	hasExternalInputs: boolean;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// Inspect raw YAML so required interpolation never needs secret values.
export function suggestSecretMappings(contents: string[], keys: string[]): SecretMappingSuggestions {
	const services = new Map<string, Map<string, unknown>>();
	let hasExternalInputs = false;
	let status: SecretMappingSuggestions['status'] = 'missing';
	try {
		for (const content of contents.filter((value) => value.trim())) {
			const doc = parseDocument(content, { merge: true });
			if (doc.errors.length || doc.warnings.length) throw new Error('Unsupported Compose YAML');
			const root: unknown = doc.toJS({ maxAliasCount: 100 });
			if (!isRecord(root)) throw new Error('Invalid Compose document');
			hasExternalInputs ||= Boolean(root['include']);
			if (root['services'] === undefined) continue;
			if (!isRecord(root['services'])) throw new Error('Invalid Compose services');
			status = 'available';
			for (const [service, config] of Object.entries(root['services'])) {
				if (!isRecord(config)) throw new Error('Invalid Compose service');
				hasExternalInputs ||= Boolean(config['env_file'] || config['extends']);
				const environment = services.get(service) ?? new Map<string, unknown>();
				const raw = config['environment'];
				if (Array.isArray(raw)) {
					for (const entry of raw) {
						if (typeof entry !== 'string') throw new Error('Invalid Compose environment');
						const separator = entry.indexOf('=');
						if (separator < 0) environment.set(entry, null);
						else environment.set(entry.slice(0, separator), entry.slice(separator + 1));
					}
				} else if (isRecord(raw)) {
					for (const [name, value] of Object.entries(raw)) {
						if (value !== null && !['string', 'number', 'boolean'].includes(typeof value))
							throw new Error('Invalid Compose environment value');
						environment.set(name, value);
					}
				} else if (raw !== undefined && raw !== null) {
					throw new Error('Invalid Compose environment');
				}
				services.set(service, environment);
			}
		}
	} catch {
		return { mappings: [], services: [], unmatchedKeys: [...new Set(keys)].sort(), status: 'invalid', hasExternalInputs };
	}
	const available = new Set(keys);
	const mappings: SecretMapping[] = [];
	for (const service of [...services.keys()].sort()) {
		for (const [name, value] of [...services.get(service)!.entries()].sort(([a], [b]) => a.localeCompare(b))) {
			if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) continue;
			let key = name;
			if (typeof value === 'string' && value.includes('$')) {
				// Composite values and escaped runtime references cannot be replaced by one secret.
				const reference = value.match(/^\$(?:\{([A-Za-z_][A-Za-z0-9_]*)(?:(?::?[-?])[^${}]*)?\}|([A-Za-z_][A-Za-z0-9_]*))$/);
				if (!reference) continue;
				key = reference[1] ?? reference[2]!;
			}
			if (available.has(key)) mappings.push({ key, name, service, delivery: 'environment' });
		}
	}
	const matched = new Set(mappings.map((mapping) => mapping.key));
	return {
		mappings,
		services: [...services.keys()].sort(),
		unmatchedKeys: [...available].filter((key) => !matched.has(key)).sort(),
		status,
		hasExternalInputs
	};
}
