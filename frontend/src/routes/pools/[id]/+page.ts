import { api, guard } from '#lib/api.ts';
import { kindKey, poolKindKey } from '#lib/format.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, depends, url }) => {
	depends('app:data');
	const filter = {
		status: url.searchParams.get('status') ?? '',
		q: url.searchParams.get('q') ?? '',
		offset: Number(url.searchParams.get('offset') ?? 0) || 0
	};
	const pool = await guard(api.pool(params.id, fetch));
	const [allocations, events, kinds] = await Promise.all([
		guard(api.allocations({ pool_id: pool.id, ...filter, limit: 25 }, fetch)),
		guard(api.events({ pool_id: pool.id, limit: 20 }, fetch)),
		guard(api.kinds(fetch))
	]);
	const kind = kinds.find((k) => kindKey(k) === poolKindKey(pool))!;
	return { pool, kind, allocations, events, filter };
};
