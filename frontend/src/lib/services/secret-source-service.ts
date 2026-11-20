import { environmentStore } from '#lib/stores/environment.store.svelte.js';
import type {
	SecretBrowse,
	SecretSetupRequest,
	SecretSetupResult,
	SecretBinding,
	SecretBindingWrite,
	SecretCheck,
	SecretOwner,
	SecretSource,
	SecretSourceWrite,
	SecretTarget
} from '#lib/types/secret-source.js';

import BaseAPIService from './api-service';

class SecretSourceService extends BaseAPIService {
	async setup(environmentId: string, body: SecretSetupRequest, operation: 'plan' | 'apply'): Promise<SecretSetupResult> {
		await environmentStore.ready;
		let path = `/agents/${environmentId}/secret-bindings/setup`;
		if (operation === 'plan') path += '/plan';
		const response = await this.api.post<{ data: SecretSetupResult }>(path, body);
		return response.data.data;
	}
	async list(): Promise<SecretSource[]> {
		const response = await this.api.get<{ data: SecretSource[] }>('/secret-sources');
		return response.data.data;
	}

	async save(id: string, body: SecretSourceWrite): Promise<SecretSource> {
		if (id) {
			const response = await this.api.put<{ data: SecretSource }>(`/secret-sources/${encodeURIComponent(id)}`, body);
			return response.data.data;
		}
		const response = await this.api.post<{ data: SecretSource }>('/secret-sources', body);
		return response.data.data;
	}

	async delete(id: string): Promise<void> {
		await this.api.delete(`/secret-sources/${encodeURIComponent(id)}`);
	}

	async test(id: string, target: SecretTarget): Promise<SecretCheck> {
		const response = await this.api.post<{ data: SecretCheck }>(`/secret-sources/${encodeURIComponent(id)}/test`, target);
		return response.data.data;
	}

	async browse(id: string, target: SecretTarget): Promise<SecretBrowse> {
		const response = await this.api.post<{ data: SecretBrowse }>(`/secret-sources/${encodeURIComponent(id)}/browse`, target);
		return response.data.data;
	}
	async keys(id: string, target: SecretTarget): Promise<SecretCheck> {
		const response = await this.api.post<{ data: SecretCheck }>(`/secret-sources/${encodeURIComponent(id)}/keys`, target);
		return response.data.data;
	}

	async bindings(environmentId: string, owner: SecretOwner): Promise<SecretBinding[]> {
		await environmentStore.ready;
		const response = await this.api.get<{ data: SecretBinding[] }>(`/agents/${environmentId}/secret-bindings`, {
			params: { kind: owner.kind, ownerId: owner.id }
		});
		return response.data.data;
	}

	async saveBinding(environmentId: string, id: string, body: SecretBindingWrite): Promise<SecretBinding> {
		await environmentStore.ready;
		const path = `/agents/${environmentId}/secret-bindings`;
		if (id) {
			const response = await this.api.put<{ data: SecretBinding }>(`${path}/${encodeURIComponent(id)}`, body);
			return response.data.data;
		}
		const response = await this.api.post<{ data: SecretBinding }>(path, body);
		return response.data.data;
	}

	async deleteBinding(environmentId: string, id: string): Promise<void> {
		await environmentStore.ready;
		await this.api.delete(`/agents/${environmentId}/secret-bindings/${encodeURIComponent(id)}`);
	}

	async apply(environmentId: string, owner: SecretOwner): Promise<void> {
		await environmentStore.ready;
		await this.api.post(`/agents/${environmentId}/secret-bindings/apply`, owner);
	}

	async saveAndRedeploy(
		environmentId: string,
		options: {
			owner: SecretOwner;
			binding?: { id: string; value: SecretBindingWrite };
			onSaved: (binding: SecretBinding) => void;
		}
	): Promise<void> {
		if (options.binding) {
			const binding = await this.saveBinding(environmentId, options.binding.id, options.binding.value);
			options.onSaved(binding);
		}
		await this.apply(environmentId, options.owner);
	}

	async check(environmentId: string, owner: SecretOwner): Promise<SecretBinding[]> {
		await environmentStore.ready;
		const response = await this.api.post<{ data: SecretBinding[] }>(`/agents/${environmentId}/secret-bindings/check`, undefined, {
			params: { kind: owner.kind, ownerId: owner.id }
		});
		return response.data.data;
	}
}

export const secretSourceService = new SecretSourceService();
