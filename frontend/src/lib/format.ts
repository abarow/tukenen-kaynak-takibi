// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

import type { KindCode, KindInfo, ReusePolicy, Status } from './types';

const nf = new Intl.NumberFormat('tr-TR');
export const num = (n: number) => nf.format(n);

export function pct(part: number, total: number): string {
	if (!total) return '%0';
	const v = (part / total) * 100;
	if (v > 0 && v < 0.1) return '<%0,1';
	return '%' + v.toLocaleString('tr-TR', { maximumFractionDigits: 1 });
}

export function dateTime(s: string | null | undefined): string {
	if (!s) return '—';
	return new Date(s).toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' });
}

const rtf = new Intl.RelativeTimeFormat('tr-TR', { numeric: 'auto' });

export function relative(s: string | null | undefined): string {
	if (!s) return '—';
	const diff = (new Date(s).getTime() - Date.now()) / 1000;
	const abs = Math.abs(diff);
	if (abs < 60) return rtf.format(Math.round(diff), 'second');
	if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
	if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
	if (abs < 86400 * 60) return rtf.format(Math.round(diff / 86400), 'day');
	return rtf.format(Math.round(diff / (86400 * 30)), 'month');
}

export function duration(seconds: number): string {
	if (seconds % 86400 === 0) return `${seconds / 86400} gün`;
	if (seconds % 3600 === 0) return `${seconds / 3600} saat`;
	if (seconds % 60 === 0) return `${seconds / 60} dk`;
	return `${seconds} sn`;
}

export const statusLabel: Record<Status | 'available' | 'reusable', string> = {
	available: 'Boşta',
	reusable: 'Tekrar kullanılabilir',
	reserved: 'Rezerve',
	assigned: 'Atanmış',
	released: 'Bırakılmış',
	blocked: 'Bloklu'
};

export const actionLabel: Record<string, string> = {
	assign: 'Atandı',
	reserve: 'Rezerve edildi',
	confirm: 'Onaylandı',
	renew: 'Rezervasyon uzatıldı',
	release: 'Bırakıldı',
	expire: 'Süresi doldu',
	block: 'Bloklandı',
	unblock: 'Blok kaldırıldı'
};

export const policyLabel: Record<ReusePolicy, string> = {
	never: 'Asla tekrar kullanma',
	cooldown: 'Karantinadan sonra',
	immediate: 'Hemen tekrar kullan'
};

export const kindShort: Record<KindCode, string> = {
	ean13: 'EAN-13',
	ean8: 'EAN-8',
	isbn13: 'ISBN-13',
	mac: 'MAC',
	custom: 'Özel',
	sequence: 'Sıra no'
};

// Özel tipler `kind = custom` ile saklanır; aralarında ayrım için birleşik anahtar kullanılır.
export const kindKey = (k: { code: KindCode; custom_kind?: string }) =>
	k.code === 'custom' ? `custom:${k.custom_kind}` : k.code;

export const poolKindKey = (p: { kind: KindCode; custom_kind: string | null }) =>
	p.kind === 'custom' ? `custom:${p.custom_kind}` : p.kind;

/** Etiket: yerleşik tiplerde kısa ad, özel tiplerde tipin adı. */
export const kindLabel = (x: { kind: KindCode; kind_name: string }) =>
	x.kind === 'custom' ? x.kind_name : kindShort[x.kind];

/** Türkçe adı URL/API kodu biçimine çevirir: "Ürün kimliği" → "urun-kimligi". */
export function slug(s: string): string {
	const map: Record<string, string> = { ı: 'i', İ: 'i', ğ: 'g', ü: 'u', ş: 's', ö: 'o', ç: 'c' };
	return s
		.replace(/[ıİğüşöç]/g, (c) => map[c])
		.toLowerCase()
		.normalize('NFD')
		.replace(/[̀-ͯ]/g, '')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 40);
}

/** Örneklemle tahmin edilmiş sayıların önüne ≈ koyar. */
export const approx = (n: number, exact = true) => (exact ? '' : '≈ ') + num(n);

// Gövde yardımcıları: onluk tipler ve onaltılık MAC adresi (48 bit, Number'a güvenle sığar).
const bodyPattern = (radix: number) => (radix === 16 ? /^[0-9A-F]+$/ : /^\d+$/);
const normalizeBody = (s: string) => s.replace(/[\s:.\-]/g, '').toUpperCase();

/** "02:4B:59:00:00:00" veya "123456" gibi bir gövde girdisini sayıya çevirir. */
export function parseBody(s: string, info: KindInfo): number | null {
	const p = normalizeBody(s);
	if (!bodyPattern(info.radix).test(p)) return null;
	const v = parseInt(p, info.radix);
	return Number.isSafeInteger(v) ? v : null;
}

/** Gövdeyi tipin yazımıyla gösterir (MAC: iki noktalı onaltılık, diğerleri: sıfır dolgulu). */
export function formatBody(n: number, info: KindInfo): string {
	if (info.radix === 16) {
		return n.toString(16).toUpperCase().padStart(info.base_digits, '0').match(/../g)!.join(':');
	}
	return String(n).padStart(info.base_digits, '0');
}

/** Sabit uzunluklu tiplerde önekin kapsadığı gövde aralığı (sunucudaki PrefixRange'in aynısı). */
export function prefixRange(prefix: string, info: KindInfo): { start: number; end: number } | null {
	const p = normalizeBody(prefix);
	if (!bodyPattern(info.radix).test(p) || p.length >= info.base_digits) return null;
	const scale = info.radix ** (info.base_digits - p.length);
	const start = parseInt(p, info.radix) * scale;
	return { start, end: start + scale - 1 };
}

export function downloadCSV(filename: string, rows: (string | number | null)[][]) {
	const esc = (v: string | number | null) => {
		const s = v === null ? '' : String(v);
		return /[",;\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
	};
	const csv = '﻿' + rows.map((r) => r.map(esc).join(';')).join('\r\n');
	const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	URL.revokeObjectURL(url);
}
