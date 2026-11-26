import { error } from '@sveltejs/kit';
import { actor } from './actor.svelte';
import type {
	AllocateRequest,
	Allocation,
	AllocationDetail,
	AllocationEvent,
	AllocationPage,
	CreatePoolInput,
	CustomKind,
	CustomKindInput,
	CustomKindTest,
	KindCode,
	KindInfo,
	LookupMatch,
	Pool,
	RangeInput,
	UpdatePoolInput,
	ValidationResult
} from './types';

export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string
	) {
		super(message);
	}
}

type Fetch = typeof fetch;

/** load fonksiyonlarında API hatasını SvelteKit hata sayfasına çevirir. */
export async function guard<T>(p: Promise<T>): Promise<T> {
	try {
		return await p;
	} catch (e) {
		if (e instanceof ApiError) error(e.status || 503, e.message);
		throw e;
	}
}

async function request<T>(method: string, path: string, body?: unknown, f: Fetch = fetch): Promise<T> {
	let res: Response;
	try {
		res = await f('/api' + path, {
			method,
			headers: {
				...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
				'X-Actor': actor.name || 'web'
			},
			body: body !== undefined ? JSON.stringify(body) : undefined
		});
	} catch {
		throw new ApiError(0, 'network', 'Sunucuya ulaşılamadı');
	}
	if (res.status === 204) return undefined as T;
	const data = await res.json().catch(() => null);
	if (!res.ok) {
		throw new ApiError(res.status, data?.error?.code ?? 'error', data?.error?.message ?? res.statusText);
	}
	return data as T;
}

function qs(params: Record<string, string | number | undefined | null>): string {
	const sp = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v !== undefined && v !== null && v !== '' && v !== 0) sp.set(k, String(v));
	}
	const s = sp.toString();
	return s ? '?' + s : '';
}

export const api = {
	kinds: (f?: Fetch) => request<KindInfo[]>('GET', '/kinds', undefined, f),

	pools: (f?: Fetch) => request<Pool[]>('GET', '/pools', undefined, f),
	pool: (id: number | string, f?: Fetch) => request<Pool>('GET', `/pools/${id}`, undefined, f),
	createPool: (input: CreatePoolInput) => request<Pool>('POST', '/pools', input),
	updatePool: (id: number, input: UpdatePoolInput) => request<Pool>('PATCH', `/pools/${id}`, input),
	deletePool: (id: number) => request<void>('DELETE', `/pools/${id}`),
	addRange: (id: number, input: RangeInput) => request<Pool>('POST', `/pools/${id}/ranges`, input),
	deleteRange: (id: number, rangeId: number) =>
		request<Pool>('DELETE', `/pools/${id}/ranges/${rangeId}`),

	allocate: (poolId: number, input: AllocateRequest) =>
		request<{ allocation: Allocation; created: boolean }>('POST', `/pools/${poolId}/allocate`, input),
	allocateBulk: (poolId: number, items: AllocateRequest[]) =>
		request<{ items: { allocation: Allocation; created: boolean }[] }>(
			'POST',
			`/pools/${poolId}/allocate/bulk`,
			{ items }
		),
	block: (poolId: number, value: string, note: string) =>
		request<Allocation>('POST', `/pools/${poolId}/block`, { value, note }),

	allocations: (
		filter: {
			pool_id?: number;
			status?: string;
			object_type?: string;
			object_ref?: string;
			q?: string;
			limit?: number;
			offset?: number;
		},
		f?: Fetch
	) => request<AllocationPage>('GET', `/allocations${qs(filter)}`, undefined, f),
	allocation: (id: number | string, f?: Fetch) =>
		request<AllocationDetail>('GET', `/allocations/${id}`, undefined, f),
	confirm: (id: number, note = '') => request<Allocation>('POST', `/allocations/${id}/confirm`, { note }),
	release: (id: number, note = '') => request<Allocation>('POST', `/allocations/${id}/release`, { note }),

	events: (filter: { pool_id?: number; limit?: number }, f?: Fetch) =>
		request<AllocationEvent[]>('GET', `/events${qs(filter)}`, undefined, f),

	validate: (kind: KindCode, value: string, prefix = '', pad_width = 0, custom_kind = '') =>
		request<ValidationResult>('POST', '/tools/validate', { kind, value, prefix, pad_width, custom_kind }),

	customKinds: (f?: Fetch) =>
		request<{ items: CustomKind[]; functions: string }>('GET', '/custom-kinds', undefined, f),
	createCustomKind: (input: CustomKindInput) => request<CustomKind>('POST', '/custom-kinds', input),
	testCustomKind: (input: CustomKindInput, value = '') =>
		request<CustomKindTest>('POST', '/custom-kinds/test', { ...input, value }),
	deleteCustomKind: (id: number) => request<void>('DELETE', `/custom-kinds/${id}`),
	lookup: (value: string) =>
		request<{ value: string; matches: LookupMatch[] }>('GET', `/lookup${qs({ value })}`)
};
