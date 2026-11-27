// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

import { api, guard } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends, url }) => {
	depends('app:data');
	const sp = url.searchParams;
	const filter = {
		status: sp.get('status') ?? '',
		q: sp.get('q') ?? '',
		offset: Number(sp.get('offset') ?? 0) || 0,
		pool_id: Number(sp.get('pool_id') ?? 0) || 0
	};
	const [allocations, pools] = await Promise.all([
		guard(api.allocations({ ...filter, limit: 50 }, fetch)),
		guard(api.pools(fetch))
	]);
	return { allocations, pools, filter };
};
