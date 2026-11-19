import assert from 'node:assert/strict';

import { afterEach, test, vi } from 'vite-plus/test';

import type { SecretBinding, SecretBindingWrite, SecretOwner } from '../types/secret-source.js';

vi.mock('#lib/stores/environment.store.svelte.js', () => ({ environmentStore: { ready: Promise.resolve() } }));
vi.mock('./api-service', () => ({ default: class {} }));

import { secretSourceService } from './secret-source-service.js';

const owner: SecretOwner = { kind: 'stack', id: 'example' };
const value: SecretBindingWrite = {
	owner,
	sourceId: 'source',
	target: { projectId: 'project', environment: 'prod', path: '/example' },
	mappings: [{ key: 'PASSWORD', name: 'DB_PASS', service: 'app', delivery: 'environment' }],
	position: 0,
	required: true,
	enabled: true,
	autoRollout: false
};
const saved: SecretBinding = { ...value, id: 'saved-binding', environmentId: 'remote', changed: false, keys: ['PASSWORD'] };

afterEach(() => vi.restoreAllMocks());

test('saves reviewed mappings and acknowledges persistence before redeploying', async () => {
	const operations: string[] = [];
	const save = vi.spyOn(secretSourceService, 'saveBinding').mockImplementation(async () => {
		operations.push('save');
		return saved;
	});
	const apply = vi.spyOn(secretSourceService, 'apply').mockImplementation(async () => {
		operations.push('redeploy');
	});
	await secretSourceService.saveAndRedeploy('remote', {
		owner,
		binding: { id: '', value },
		onSaved: (binding) => {
			assert.equal(binding.id, 'saved-binding');
			operations.push('acknowledge');
		}
	});
	assert.deepEqual(operations, ['save', 'acknowledge', 'redeploy']);
	assert.deepEqual(save.mock.calls, [['remote', '', value]]);
	assert.deepEqual(apply.mock.calls, [['remote', owner]]);
});

test('does not redeploy or acknowledge saved mappings if saving fails', async () => {
	const failure = new Error('Permission denied');
	vi.spyOn(secretSourceService, 'saveBinding').mockRejectedValue(failure);
	const apply = vi.spyOn(secretSourceService, 'apply').mockResolvedValue();
	const onSaved = vi.fn();
	await assert.rejects(secretSourceService.saveAndRedeploy('remote', { owner, binding: { id: '', value }, onSaved }), failure);
	assert.equal(apply.mock.calls.length, 0);
	assert.equal(onSaved.mock.calls.length, 0);
});

test('retains the saved identity when redeployment fails and retries without creating a duplicate binding', async () => {
	const save = vi.spyOn(secretSourceService, 'saveBinding').mockResolvedValue(saved);
	const apply = vi.spyOn(secretSourceService, 'apply').mockRejectedValueOnce(new Error('Agent unavailable')).mockResolvedValue();
	let persisted: SecretBinding | undefined;
	await assert.rejects(
		secretSourceService.saveAndRedeploy('remote', {
			owner,
			binding: { id: '', value },
			onSaved: (binding) => {
				persisted = binding;
			}
		}),
		/Agent unavailable/
	);
	assert.equal(persisted?.id, 'saved-binding');
	await secretSourceService.saveAndRedeploy('remote', {
		owner,
		onSaved: () => {
			assert.fail('Existing mappings must not be saved again');
		}
	});
	assert.equal(save.mock.calls.length, 1);
	assert.equal(apply.mock.calls.length, 2);
});

test('updates the selected binding before applying the workload secrets', async () => {
	const save = vi.spyOn(secretSourceService, 'saveBinding').mockResolvedValue(saved);
	vi.spyOn(secretSourceService, 'apply').mockResolvedValue();
	await secretSourceService.saveAndRedeploy('remote', { owner, binding: { id: 'saved-binding', value }, onSaved: () => {} });
	assert.equal(save.mock.calls[0]?.[1], 'saved-binding');
});
