import { ApiError } from './api';

export interface Toast {
	id: number;
	kind: 'ok' | 'error';
	text: string;
}

export const toasts = $state<Toast[]>([]);
let next = 1;

function push(kind: Toast['kind'], text: string) {
	const id = next++;
	toasts.push({ id, kind, text });
	setTimeout(() => dismiss(id), kind === 'error' ? 7000 : 3500);
}

export function dismiss(id: number) {
	const i = toasts.findIndex((t) => t.id === id);
	if (i >= 0) toasts.splice(i, 1);
}

export const notify = {
	ok: (text: string) => push('ok', text),
	error: (err: unknown) =>
		push('error', err instanceof ApiError || err instanceof Error ? err.message : String(err))
};
