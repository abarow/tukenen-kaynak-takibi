import { api, guard } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('app:data');
	const [pools, kinds] = await Promise.all([guard(api.pools(fetch)), guard(api.kinds(fetch))]);
	return { pools, kinds };
};
