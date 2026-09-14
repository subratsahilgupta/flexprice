# TypeScript SDK JWT Authentication Design

## Summary

Add JWT authentication and optional environment selection to the generated TypeScript SDK while preserving the existing API-key interface. Keep the canonical OpenAPI document and the Go, Python, and MCP generation targets unchanged.

The SDK will support dynamic credentials so browser applications can provide the latest session token and selected environment before each request. Token acquisition and refresh remain the caller's responsibility.

## Goals

- Keep `apiKeyAuth` working for server-side integrations.
- Add bearer JWT authentication to `@flexprice/sdk`.
- Allow, but do not require, `X-Environment-ID` with JWT authentication.
- Read JWT and environment values dynamically before each protected request.
- Keep public endpoints unauthenticated.
- Limit the generated contract change to the TypeScript SDK.

## Non-goals

- Migrating the FlexPrice frontend from its raw API client.
- Changing backend authentication or environment-resolution behavior.
- Adding JWT support to the Go, Python, or MCP outputs.
- Refreshing tokens inside the SDK.
- Adding client-side validation for conflicting credentials or environment ownership.

## Current State

`cmd/server/main.go` declares only `ApiKeyAuth` in the Swagger metadata, and protected handlers use `@Security ApiKeyAuth`. The resulting OpenAPI document therefore gives Speakeasy only an API-key security scheme.

The backend already supports both credential types in `internal/rest/middleware/auth.go`. It checks `x-api-key` first and falls back to a bearer token. For JWT requests, the environment comes from the token claim when present; otherwise the middleware uses `X-Environment-ID`. If neither source selects an environment for an endpoint that needs one, the existing response is `403 Forbidden`.

## Generation Architecture

Create `.speakeasy/overlays/typescript-auth.yaml` and apply it only to a new TypeScript-specific source in `.speakeasy/workflow.yaml`. That source will apply the existing shared SDK overlay first and the TypeScript authentication overlay second. Only the `flexprice-typescript` target will use the new source.

The base `swagger-json` source remains unchanged for Go and Python. The filtered MCP source and target remain unchanged.

The TypeScript overlay will:

1. Add an HTTP bearer security scheme for the `Authorization` header.
2. Add a header security scheme for `X-Environment-ID` so it can participate in the same dynamic security callback.
3. Replace each operation security declaration that currently uses `ApiKeyAuth` with these alternatives, in order:
   - API key
   - bearer JWT plus environment ID
   - bearer JWT alone
4. Leave operations without an existing security declaration unchanged.

Putting the bearer-plus-environment alternative before bearer-only ensures Speakeasy selects the form that sends `X-Environment-ID` when the callback supplies it, while still allowing a JWT without that header.

## Generated TypeScript Interface

The generated SDK will accept either static security values or Speakeasy's supported asynchronous security callback. The documented frontend pattern will use the callback:

```ts
const flexprice = new FlexPrice({
  security: async () => ({
    bearerAuth: await getCurrentToken(),
    environmentId: getSelectedEnvironmentId() ?? undefined,
  }),
});
```

The exact exported security type and generated property casing will be verified after generation and used in the final examples. No handwritten request hook will be added.

API-key users retain the existing initialization path. Callers should choose one credential mode. The SDK adds no special rejection when a caller provides more than one mode; the backend's existing API-key-first authentication order is not changed.

## Request Flow

For every protected request:

1. Speakeasy invokes the configured security callback.
2. The caller returns its current credentials.
3. The generated client serializes the JWT as `Authorization: Bearer <token>`.
4. When supplied, it serializes the selected environment as `X-Environment-ID: <environment-id>`.
5. The backend authenticates the request and resolves the environment using its existing middleware.

Changing the callback's token or environment source affects the next request without reconstructing the SDK client.

## Errors and Edge Cases

- An expired or invalid JWT produces the backend's existing `401` response and generated SDK error.
- A missing environment ID is valid at the SDK layer. The backend may resolve it from the JWT claim or return its existing `403` response.
- An invalid or cross-tenant environment ID is rejected by the backend as it is today.
- The SDK does not inspect JWT claims, own session state, refresh tokens, or validate environment access.
- API keys must not be embedded in browser applications; the JWT example will be the documented browser-safe path.

## Persistent Files

Expected implementation changes are limited to:

- `.speakeasy/overlays/typescript-auth.yaml`
- `.speakeasy/workflow.yaml`
- `.speakeasy/README.md`
- `api/custom/typescript/README.md`
- `api/custom/typescript/examples/quick-start.ts`
- `api/custom/typescript/examples/.env.sample`
- generation-contract or TypeScript request tests needed to keep the behavior stable

Generated files under `api/typescript/` remain generator output rather than the source of custom behavior.

## Verification

Verification will cover:

1. Validate the TypeScript overlay.
2. Apply the shared and TypeScript overlays to a temporary specification and validate the result.
3. Assert that representative protected operations expose API-key, bearer-plus-environment, and bearer-only alternatives.
4. Assert that representative public login and signup operations remain unauthenticated.
5. Generate only the TypeScript target for local verification.
6. Run the generated TypeScript package's build and tests.
7. Use a mocked HTTP client to verify:
   - API-key requests send `x-api-key`.
   - JWT requests send the bearer `Authorization` header.
   - An environment value sends `X-Environment-ID`.
   - Omitting the environment omits that header.
   - Updated callback values are observed by the next request.

No backend behavior changes are required. Existing middleware tests remain the source of truth for JWT validation and tenant-scoped environment resolution.

## Release Boundary

The repository's release workflow may continue to generate and version every SDK artifact together. This change only alters the TypeScript target's generated authentication contract. Publishing and frontend adoption happen through the existing release process; frontend migration is a separate follow-up.
