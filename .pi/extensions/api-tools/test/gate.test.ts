import { mkdir, mkdtemp, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { describe, expect, it } from 'vitest';
import apiToolsExtension from '../index.js';
import { API_TOOL_NAMES } from '../src/tools.js';
import type { ApiClient, ApiToolsConfig } from '../src/types.js';

// The project opts into this extension through a committed `.pi/extensions.json`
// holding `{"api-tools": true}`. That gate is what the CanchaLibre verification
// harness (docs/roadmap-backend.md section 5.4) depends on: without it the
// extension registers nothing and `api_swagger` reports MISSING at runtime, which
// leaves the contract unverifiable. A clean clone had no such file, so this test
// pins the gate from both sides.

function createMockPi() {
  const tools: any[] = [];
  return {
    tools,
    registerTool(tool: any) {
      tools.push(tool);
    },
    registerToolRenderer() {},
    on() {},
  };
}

function createConfig(overrides: Partial<ApiToolsConfig> = {}): ApiToolsConfig {
  return {
    configPath: '/tmp/project/.pi/api.json',
    exists: true,
    enabled: true,
    url: 'https://api.example.test/base/',
    swagger: { configured: true, enabled: true, framework: 'openapi', valid: true },
    graphql: { configured: false, enabled: false, framework: 'spring', valid: true },
    headers: {},
    auth: { type: 'none' },
    timeoutMs: 30000,
    limits: { maxResponseBytes: 50000, maxResponseLines: 2000, cursorTtlSeconds: 3600 },
    warnings: [],
    secretValues: [],
    git: { state: 'unknown' },
    ...overrides,
  };
}

function createMockClient(): ApiClient {
  return {
    login: async () => ({ status: 200, statusText: 'OK', headers: {}, bodyText: '{}' }),
    rest: async () => ({ status: 200, statusText: 'OK', headers: {}, bodyText: '{}' }),
    graphql: async () => ({ status: 200, statusText: 'OK', headers: {}, bodyText: '{}' }),
  } as unknown as ApiClient;
}

/** A project root with `.pi/` present and `extensions.json` written only when given. */
async function projectWith(extensionsJson: string | null): Promise<string> {
  const cwd = await mkdtemp(join(tmpdir(), 'api-tools-gate-'));
  await mkdir(join(cwd, '.pi'), { recursive: true });
  if (extensionsJson !== null) {
    await writeFile(join(cwd, '.pi', 'extensions.json'), extensionsJson, 'utf8');
  }
  return cwd;
}

async function registerIn(cwd: string, config: ApiToolsConfig = createConfig()) {
  const pi = createMockPi();
  await apiToolsExtension(pi, { cwd, loadConfig: async () => config, client: createMockClient() });
  return pi.tools.map((tool) => tool.name);
}

describe('project enablement gate', () => {
  it('registers nothing when .pi/extensions.json is absent, even with a valid api.json', async () => {
    // The regression: a configured api.json alone must not enable the tools.
    expect(await registerIn(await projectWith(null))).toEqual([]);
  });

  it('registers the tools when the flag is exactly true', async () => {
    const names = await registerIn(await projectWith(JSON.stringify({ 'api-tools': true })));
    expect(names).toContain('api_swagger');
    expect(names).toContain('api_status');
    expect(names).toContain('api_login');
    // api_graphql is conditional on graphql being enabled in api.json
    // (src/tools.ts), which CanchaLibre's api.json does not enable. That is why
    // the section 5.4 subagent allowlists five of the six tools.
    expect(names).toEqual(['api_status', 'api_auth_status', 'api_login', 'api_rest_request', 'api_swagger']);
  });

  it('adds api_graphql only when GraphQL is enabled in the API config', async () => {
    const withGraphql = createConfig({
      graphql: { configured: true, enabled: true, framework: 'spring', valid: true },
    });
    const names = await registerIn(await projectWith(JSON.stringify({ 'api-tools': true })), withGraphql);
    expect(names).toContain('api_graphql');
    expect(names).toHaveLength([...API_TOOL_NAMES].length);
  });

  it('stays inert when the flag is false, or malformed, or the file is not JSON', async () => {
    expect(await registerIn(await projectWith(JSON.stringify({ 'api-tools': false })))).toEqual([]);
    expect(await registerIn(await projectWith(JSON.stringify({ other: true })))).toEqual([]);
    expect(await registerIn(await projectWith('{ not json'))).toEqual([]);
  });
});
