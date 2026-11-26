import { api, guard } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, depends }) => {
	depends('app:data');
	return { allocation: await guard(api.allocation(params.id, fetch)) };
};
