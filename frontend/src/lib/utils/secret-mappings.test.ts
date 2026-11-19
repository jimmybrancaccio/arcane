import assert from 'node:assert/strict';

import { test } from 'vite-plus/test';

import { suggestSecretMappings } from './secret-mappings.js';

test('scopes shared and renamed secrets to only services declaring them', () => {
	const result = suggestSecretMappings(
		[
			`
services:
  web:
    environment:
      DB_PASS: \${DB_PASS:?required}
      AUTH_TOKEN: \${RENDERER_TOKEN}
      LOCAL_ONLY: public
  worker:
    environment:
      - DB_PASS
      - SMTP_PASSWORD=$SMTP_PASSWORD
  cache:
    image: redis
`
		],
		['DB_PASS', 'RENDERER_TOKEN', 'SMTP_PASSWORD', 'UNUSED']
	);
	assert.equal(result.status, 'available');
	assert.deepEqual(result.mappings, [
		{ key: 'RENDERER_TOKEN', name: 'AUTH_TOKEN', service: 'web', delivery: 'environment' },
		{ key: 'DB_PASS', name: 'DB_PASS', service: 'web', delivery: 'environment' },
		{ key: 'DB_PASS', name: 'DB_PASS', service: 'worker', delivery: 'environment' },
		{ key: 'SMTP_PASSWORD', name: 'SMTP_PASSWORD', service: 'worker', delivery: 'environment' }
	]);
	assert.deepEqual(result.unmatchedKeys, ['UNUSED']);
	assert.ok(result.mappings.every((mapping) => Boolean(mapping.service)));
});

test('resolves YAML environment anchors and merges overrides by destination', () => {
	const result = suggestSecretMappings(
		[
			`
x-env: &env
  PASSWORD: \${OLD_PASSWORD}
  SHARED: ''
services:
  app:
    environment:
      <<: *env
  worker:
    environment: *env
`,
			`
services:
  app:
    environment:
      PASSWORD: \${NEW_PASSWORD:-fallback}
`
		],
		['OLD_PASSWORD', 'NEW_PASSWORD', 'SHARED']
	);
	assert.deepEqual(
		result.mappings.map(({ service, name, key }) => [service, name, key]),
		[
			['app', 'PASSWORD', 'NEW_PASSWORD'],
			['app', 'SHARED', 'SHARED'],
			['worker', 'PASSWORD', 'OLD_PASSWORD'],
			['worker', 'SHARED', 'SHARED']
		]
	);
});

test('does not replace composite, escaped, conditional or unknown references', () => {
	const result = suggestSecretMappings(
		[
			`
services:
  app:
    environment:
      DSN: mysql://user:\${PASSWORD}@database/app
      ESCAPED: $$ESCAPED
      CONDITIONAL: \${PASSWORD:+present}
      UNKNOWN: \${MISSING}
      NESTED: \${PASSWORD:-\${FALLBACK}}
      LITERAL: old-secret
      EMPTY:
`
		],
		['PASSWORD', 'DSN', 'ESCAPED', 'CONDITIONAL', 'UNKNOWN', 'NESTED', 'LITERAL', 'EMPTY']
	);
	assert.deepEqual(
		result.mappings.map(({ key }) => key),
		['EMPTY', 'LITERAL']
	);
});

test('reports missing, malformed and unsupported Compose without broad fallback', () => {
	for (const content of [
		'',
		'services: [',
		'services: { app: { environment: !reset {} } }',
		'services: {app: {environment: 42}}'
	]) {
		const result = suggestSecretMappings([content], ['PASSWORD']);
		assert.equal(result.status, content ? 'invalid' : 'missing');
		assert.deepEqual(result.mappings, []);
		assert.deepEqual(result.unmatchedKeys, ['PASSWORD']);
	}
});

test('flags external inputs and leaves their secrets unselected', () => {
	const result = suggestSecretMappings(
		[
			`
include: extra.yml
services:
  app:
    extends: {file: base.yml, service: base}
    env_file: .env
    environment: {DIRECT: ''}
`
		],
		['DIRECT', 'EXTERNAL']
	);
	assert.equal(result.hasExternalInputs, true);
	assert.deepEqual(result.unmatchedKeys, ['EXTERNAL']);
});

test('deduplicates available keys and honors last list entry and later overrides', () => {
	const result = suggestSecretMappings(
		[
			`
services:
  app:
    environment: ['TOKEN=\${FIRST}', 'TOKEN=\${SECOND}']
`,
			`services: {app: {environment: {TOKEN: null}}}`
		],
		['FIRST', 'SECOND', 'TOKEN', 'TOKEN']
	);
	assert.deepEqual(result.mappings, [{ key: 'TOKEN', name: 'TOKEN', service: 'app', delivery: 'environment' }]);
	assert.deepEqual(result.unmatchedKeys, ['FIRST', 'SECOND']);
});

test('accepts non-service override files and ignores unrelated Compose fields', () => {
	const result = suggestSecretMappings(
		[
			'services: {app: {image: "${IMAGE}", environment: {TOKEN: ""}, labels: {PASSWORD: "${PASSWORD}"}}}',
			'networks: {default: {external: true}}'
		],
		['IMAGE', 'TOKEN', 'PASSWORD']
	);
	assert.equal(result.status, 'available');
	assert.deepEqual(
		result.mappings.map(({ key }) => key),
		['TOKEN']
	);
	assert.deepEqual(result.unmatchedKeys, ['IMAGE', 'PASSWORD']);
});
