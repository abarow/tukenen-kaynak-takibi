<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { untrack } from 'svelte';
	import { approx, formatBody, kindKey, num, parseBody, pct, policyLabel, prefixRange, slug } from '../format';
	import type { KindCode, KindInfo, RangeInput, ReusePolicy } from '../types';

	let {
		kinds,
		initialKind = null,
		oncancel
	}: { kinds: KindInfo[]; initialKind?: string | null; oncancel: () => void } = $props();

	let name = $state('');
	let code = $state('');
	let codeTouched = $state(false);
	let description = $state('');
	// Seçim anahtarı: yerleşik tipte kod ("ean13"), özel tipte "custom:<kod>".
	let selected = $state(
		untrack(() => (initialKind && kinds.some((k) => kindKey(k) === initialKind) ? initialKind : 'ean13'))
	);
	let seqPrefix = $state('');
	let padWidth = $state(6);
	let policy = $state<ReusePolicy>('never');
	let cooldownDays = $state(1460);
	let ttlMinutes = $state(15);
	let lowWatermark = $state(0);
	let rangeMode = $state<'prefix' | 'explicit'>('prefix');
	let rangePrefix = $state('');
	let rangeStart = $state('');
	let rangeEnd = $state('');
	let busy = $state(false);

	let info = $derived(kinds.find((k) => kindKey(k) === selected)!);
	let kind = $derived(info.code);
	let fixedLength = $derived(info.base_digits > 0);
	let mode = $derived(fixedLength ? rangeMode : 'explicit');
	let builtins = $derived(kinds.filter((k) => k.code !== 'custom'));
	let customs = $derived(kinds.filter((k) => k.code === 'custom'));

	const placeholders: Record<KindCode, string> = {
		ean13: 'GS1 firma öneki, ör. 8691234',
		ean8: 'ör. 963',
		isbn13: 'Yayıncı öneki, ör. 978-605-4567',
		mac: 'IEEE bloğu: MA-L 00:00:5E · MA-M 00:00:5E:0 · MA-S 00:00:5E:00:5',
		sequence: '',
		custom: 'gövde öneki, ör. 246 → 246xxxxx'
	};

	$effect(() => {
		if (!codeTouched) code = slug(name);
	});

	// Tip değişince o tipe uygun varsayılan politikayı öner.
	function onKindChange() {
		policy = info.default_reuse_policy;
		if (!fixedLength) rangeMode = 'explicit';
	}

	let preview = $derived.by(() => {
		if (mode === 'prefix') {
			if (!rangePrefix) return null;
			const r = prefixRange(rangePrefix, info);
			const chars = info.radix === 16 ? 'onaltılık (0-9, A-F)' : 'yalnızca rakam';
			return r
				? { ...r, size: r.end - r.start + 1 }
				: { error: `Önek ${info.base_digits} haneden kısa ve ${chars} olmalı` };
		}
		if (rangeStart === '' || rangeEnd === '') return null;
		const s = parseBody(rangeStart, info);
		const e = parseBody(rangeEnd, info);
		if (s === null || e === null || s > e) return { error: 'Geçersiz aralık' };
		return { start: s, end: e, size: e - s + 1 };
	});

	async function submit(ev: SubmitEvent) {
		ev.preventDefault();
		const ranges: RangeInput[] = [];
		if (mode === 'prefix' && rangePrefix) ranges.push({ prefix: rangePrefix });
		if (mode === 'explicit' && rangeStart !== '' && rangeEnd !== '') {
			if (!preview || 'error' in preview) {
				notify.error('Aralık başlangıç/bitiş değerleri geçersiz');
				return;
			}
			ranges.push({ start: preview.start, end: preview.end });
		}
		busy = true;
		try {
			const p = await api.createPool({
				code,
				name,
				description,
				kind,
				custom_kind: info.custom_kind,
				prefix: kind === 'sequence' ? seqPrefix : '',
				pad_width: kind === 'sequence' ? padWidth : 0,
				reuse_policy: policy,
				cooldown_days: policy === 'cooldown' ? cooldownDays : 0,
				reservation_ttl_seconds: Math.round(ttlMinutes * 60),
				low_watermark: lowWatermark,
				ranges
			});
			notify.ok(`“${p.name}” oluşturuldu`);
			await goto(`/pools/${p.id}`);
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}
</script>

<form onsubmit={submit}>
	<h2>Yeni havuz</h2>

	<div class="row">
		<div class="field">
			<label for="name">Ad</label>
			<input id="name" bind:value={name} required placeholder="ör. Ana ürün barkodları" />
		</div>
		<div class="field">
			<label for="code">Kod</label>
			<input
				id="code"
				class="mono"
				bind:value={code}
				oninput={() => (codeTouched = true)}
				required
				pattern="[a-z0-9][a-z0-9_\-]+"
			/>
			<div class="hint">API'de kullanılır: /api/pools/{code || 'kod'}/allocate</div>
		</div>
	</div>

	<div class="field">
		<label for="kind">Kaynak tipi</label>
		<select id="kind" bind:value={selected} onchange={onKindChange}>
			<optgroup label="Hazır tipler">
				{#each builtins as k (kindKey(k))}
					<option value={kindKey(k)}>{k.name}</option>
				{/each}
			</optgroup>
			{#if customs.length}
				<optgroup label="Özel tipler">
					{#each customs as k (kindKey(k))}
						<option value={kindKey(k)}>{k.name}</option>
					{/each}
				</optgroup>
			{/if}
		</select>
		<div class="hint">
			{info.description} Örnek: <span class="mono">{info.example}</span> ·
			<a href="/kinds?new=1">kendi kod tipini tanımla</a>
		</div>
	</div>

	{#if kind === 'sequence'}
		<div class="row">
			<div class="field">
				<label for="sp">Önek</label>
				<input id="sp" class="mono" bind:value={seqPrefix} placeholder="ör. FTR-" maxlength="16" />
			</div>
			<div class="field">
				<label for="pad">Sıfır dolgusu (hane)</label>
				<input id="pad" type="number" min="0" max="18" bind:value={padWidth} />
				<div class="hint">
					Görünüm: <span class="mono">{seqPrefix}{String(1).padStart(padWidth, '0')}</span>
				</div>
			</div>
		</div>
	{/if}

	<fieldset>
		<legend>İlk aralık <span class="muted">(sonra da eklenebilir)</span></legend>
		{#if fixedLength}
			<div class="segmented" style="margin-bottom: 10px">
				<button type="button" class:on={rangeMode === 'prefix'} onclick={() => (rangeMode = 'prefix')}>
					Önekten
				</button>
				<button type="button" class:on={rangeMode === 'explicit'} onclick={() => (rangeMode = 'explicit')}>
					Başlangıç – bitiş
				</button>
			</div>
		{/if}
		{#if mode === 'prefix'}
			<div class="field">
				<label for="rp">Önek</label>
				<input id="rp" class="mono" bind:value={rangePrefix} placeholder={placeholders[kind]} />
			</div>
		{:else}
			<div class="row">
				<div class="field">
					<label for="rs">Başlangıç</label>
					<input id="rs" class="mono" inputmode={info.radix === 16 ? 'text' : 'numeric'} bind:value={rangeStart} />
				</div>
				<div class="field">
					<label for="re">Bitiş</label>
					<input id="re" class="mono" inputmode={info.radix === 16 ? 'text' : 'numeric'} bind:value={rangeEnd} />
				</div>
			</div>
			{#if info.radix === 16}
				<div class="hint">Tam adres girin, ör. 02:4B:59:00:00:00 – 02:4B:59:00:FF:FF. Aralık tek bir ilk oktet içinde kalmalı.</div>
			{:else if fixedLength}
				<div class="hint">
					Kontrol hanesi hariç {info.base_digits} haneli gövde değerleri girin; kontrol haneleri otomatik
					hesaplanır.
				</div>
			{/if}
		{/if}
		{#if preview}
			{#if 'error' in preview}
				<div class="hint" style="color: var(--danger)">{preview.error}</div>
			{:else}
				<div class="hint">
					<span class="mono">{formatBody(preview.start, info)}</span> –
					<span class="mono">{formatBody(preview.end, info)}</span>
					{#if info.density}
						→ {num(preview.size)} gövde, kurallara uyan
						<strong>{approx(Math.round(preview.size * info.density), false)}</strong> kod
						<span class="muted">(gövdelerin {pct(info.density, 1)} kadarı; kesin sayı kayıtta hesaplanır)</span>
					{:else}
						→ <strong>{num(preview.size)}</strong> değer
					{/if}
				</div>
			{/if}
		{/if}
	</fieldset>

	<div class="row">
		<div class="field">
			<label for="policy">Bırakılan değerin yeniden kullanımı</label>
			<select id="policy" bind:value={policy}>
				{#each Object.entries(policyLabel) as [k, v] (k)}
					<option value={k}>{v}</option>
				{/each}
			</select>
			<div class="hint">
				ISBN, GTIN ve cihaza basılan MAC adresleri tekrar kullanılmaz. Sanal makine MAC havuzlarında
				karantinalı tekrar kullanım uygundur.
			</div>
		</div>
		{#if policy === 'cooldown'}
			<div class="field">
				<label for="cd">Karantina (gün)</label>
				<input id="cd" type="number" min="1" bind:value={cooldownDays} />
			</div>
		{/if}
		<div class="field">
			<label for="ttl">Rezervasyon süresi (dk)</label>
			<input id="ttl" type="number" min="1" bind:value={ttlMinutes} />
		</div>
		<div class="field">
			<label for="low">Uyarı eşiği (boşta)</label>
			<input id="low" type="number" min="0" bind:value={lowWatermark} />
			<div class="hint">0 = uyarı yok</div>
		</div>
	</div>

	<div class="field">
		<label for="desc">Açıklama</label>
		<input id="desc" bind:value={description} placeholder="opsiyonel" />
	</div>

	<div class="row" style="justify-content: flex-end">
		<button type="button" onclick={oncancel}>Vazgeç</button>
		<button class="primary" disabled={busy}>Oluştur</button>
	</div>
</form>

<style>
	fieldset {
		border: 1px dashed var(--border);
		border-radius: var(--radius);
		padding: 12px 14px 4px;
		margin: 0 0 12px;
	}
	legend {
		font-size: 12px;
		font-weight: 600;
		padding: 0 6px;
	}
</style>
