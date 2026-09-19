import type { TokenInfo } from '$lib/types';

import { config, type ClientCredentials } from './config';

type CachedToken = {
	accessToken: string;
	expiresAt: number;
	info: TokenInfo;
};

const cache = new Map<string, CachedToken>();

/** Refresh tokens a few seconds before they expire. */
const EXPIRY_MARGIN_MS = 5_000;

/** Requests a new client_credentials token from Keycloak. */
async function requestToken(client: ClientCredentials): Promise<CachedToken> {
	const response = await fetch(config.tokenUrl, {
		method: 'POST',
		headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
		body: new URLSearchParams({
			grant_type: 'client_credentials',
			client_id: client.clientId,
			client_secret: client.clientSecret
		}),
		signal: AbortSignal.timeout(5_000)
	});

	if (!response.ok) {
		throw new Error(`Keycloak rejected ${client.clientId}: HTTP ${response.status}`);
	}

	const body = (await response.json()) as { access_token: string; expires_in: number };

	return {
		accessToken: body.access_token,
		expiresAt: Date.now() + body.expires_in * 1_000 - EXPIRY_MARGIN_MS,
		info: describe(body.access_token, body.expires_in)
	};
}

/** Reads the claims of a JWT for display only; the API is the one that validates it. */
function describe(token: string, expiresIn: number): TokenInfo {
	const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString()) as Record<string, unknown>;

	return {
		issuedAt: new Date().toISOString(),
		expiresIn,
		claims: {
			iss: payload.iss,
			aud: payload.aud,
			azp: payload.azp,
			typ: payload.typ,
			provider_id: payload.provider_id,
			roles: (payload.realm_access as { roles?: string[] } | undefined)?.roles,
			exp: payload.exp
		}
	};
}

/** Returns a cached access token, requesting a new one when it is about to expire. */
export async function accessToken(client: ClientCredentials): Promise<string> {
	const cached = cache.get(client.clientId);
	if (cached && cached.expiresAt > Date.now()) {
		return cached.accessToken;
	}

	const token = await requestToken(client);
	cache.set(client.clientId, token);

	return token.accessToken;
}

/** Requests a fresh token and returns only its claims; the token itself never reaches the browser. */
export async function issueToken(client: ClientCredentials): Promise<TokenInfo> {
	const token = await requestToken(client);
	cache.set(client.clientId, token);

	return token.info;
}

/** Claims of the cached token, if there is one. */
export function currentToken(client: ClientCredentials): TokenInfo | undefined {
	const cached = cache.get(client.clientId);

	return cached && cached.expiresAt > Date.now() ? cached.info : undefined;
}

export type ProviderStatus = 'RUNNING' | 'INACTIVE';

/** A provider is RUNNING when its realm client can obtain an access token. */
export async function providerStatus(client: ClientCredentials): Promise<{ status: ProviderStatus; reason?: string }> {
	try {
		await accessToken(client);

		return { status: 'RUNNING' };
	} catch (error) {
		return { status: 'INACTIVE', reason: error instanceof Error ? error.message : String(error) };
	}
}
