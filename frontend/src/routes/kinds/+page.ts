import { api, guard } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('app:data');
	const [kinds, custom] = await Promise.all([guard(api.kinds(fetch)), guard(api.customKinds(fetch))]);
	return { builtins: kinds.filter((k) => k.code !== 'custom'), custom: custom.items };
};
