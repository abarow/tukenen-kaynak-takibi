// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

import { api, guard } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('app:data');
	const [pools, events] = await Promise.all([
		guard(api.pools(fetch)),
		guard(api.events({ limit: 15 }, fetch))
	]);
	return { pools, events };
};
