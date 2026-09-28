import type { BeforeRequestHook } from "./types.js";

type Credential = string | (() => string | undefined | Promise<string | undefined>) | undefined;

async function resolveCredential(value: Credential): Promise<string | undefined> {
  const resolved = typeof value === "function" ? await value() : value;
  return resolved || undefined;
}

/** Sends bearerAuth and environmentId as headers. Headers passed on a call take precedence. */
export const authHeadersHook: BeforeRequestHook = {
  async beforeRequest(hookCtx, request) {
    const { apiKeyAuth, bearerAuth, environmentId } = hookCtx.options;
    if (apiKeyAuth && bearerAuth) {
      throw new Error("set either apiKeyAuth or bearerAuth, not both.");
    }

    const token = await resolveCredential(bearerAuth);
    if (token && !request.headers.has("Authorization")) {
      request.headers.set("Authorization", `Bearer ${token}`);
    }

    const envId = await resolveCredential(environmentId);
    if (envId && !request.headers.has("X-Environment-ID")) {
      request.headers.set("X-Environment-ID", envId);
    }

    return request;
  },
};
