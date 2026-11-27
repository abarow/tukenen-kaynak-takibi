<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { num, pct, slug } from '../format';
	import type { CustomKindInput, CustomKindTest } from '../types';

	let { onclose }: { onclose: () => void } = $props();

	let form = $state<CustomKindInput>({
		code: '',
		name: '',
		description: '',
		body_digits: 8,
		check_digits: 4,
		rules: [{ expr: '', message: '' }],
		check_expr: ''
	});
	let codeTouched = $state(false);
	let tryValue = $state('');
	let result = $state<CustomKindTest | null>(null);
	let busy = $state(false);

	$effect(() => {
		if (!codeTouched) form.code = slug(form.name);
	});

	// Formül yazmayı öğretmek için hazır örnekler.
	const examples = {
		urun: {
			label: 'Ürün kimliği',
			note: '',
			def: {
				code: 'urun-kimligi',
				name: 'Ürün kimliği',
				description: 'Kurallı 12 haneli ürün kimliği',
				body_digits: 8,
				check_digits: 4,
				rules: [
					{ expr: 'digitSum(1, 3) % 2 == 0', message: 'ilk 3 hanenin toplamı çift olmalı' },
					{ expr: 'digitSum(4, 8) % 3 == 0', message: "sonraki 5 hanenin toplamı 3'e bölünmeli" }
				],
				check_expr: 'sum(chunks(1, 8, 2))'
			}
		},
		isbn: {
			label: 'ISBN-13',
			// Özel tipler kendi uzaylarında tutulur; hazır ISBN havuzlarıyla çakışma denetlenmez.
			note:
				'ISBN-13 hazır tip olarak zaten var. Bu örnek, aynı algoritmanın formüllerle nasıl yazıldığını gösterir. ' +
				'Gerçek ISBN havuzları için hazır tipi kullanın: özel tipler hazır ISBN havuzlarıyla çakışma denetimine girmez.',
			def: {
				code: 'isbn-formul',
				name: 'ISBN-13 (formülle)',
				description: 'Kitap numarası: 978/979 + grup + yayıncı + yayın no + GS1 kontrol hanesi',
				body_digits: 12,
				check_digits: 1,
				rules: [{ expr: 'number(1, 3) == 978 or number(1, 3) == 979', message: 'ISBN 978 veya 979 ile başlamalı' }],
				check_expr: '(10 - weightedSum(1, 12, [1, 3]) % 10) % 10'
			}
		}
	} satisfies Record<string, { label: string; note: string; def: CustomKindInput }>;

	let exampleNote = $state('');

	function fillExample(key: keyof typeof examples) {
		const ex = examples[key];
		form = structuredClone(ex.def);
		exampleNote = ex.note;
		codeTouched = true;
	}

	function payload(): CustomKindInput {
		return {
			...form,
			rules: form.rules.filter((r) => r.expr.trim() !== ''),
			check_expr: form.check_digits > 0 ? form.check_expr : ''
		};
	}

	// Yazarken canlı dene (kaydetmeden): derleme hataları, yoğunluk ve örnek kodlar.
	let timer: ReturnType<typeof setTimeout> | undefined;
	// Her sonuç hangi tanım için hesaplandığını taşır: geç gelen eski yanıtlar yok sayılır,
	// tanım değiştiyse eski sonuç soluk gösterilir ve kaydetme kapanır.
	let current = $derived(JSON.stringify({ ...payload(), value: tryValue }));
	let resultKey = $state('');
	let resultDigits = $state(0);
	let fresh = $derived(result !== null && resultKey === current);

	$effect(() => {
		const key = current;
		const input = payload();
		const value = tryValue;
		clearTimeout(timer);
		timer = setTimeout(async () => {
			let r: CustomKindTest;
			try {
				r = await api.testCustomKind(input, value);
			} catch (e) {
				r = { ok: false, error: e instanceof Error ? e.message : String(e) };
			}
			if (key !== current) return; // bu arada tanım değişti; eski yanıt
			result = r;
			resultKey = key;
			resultDigits = input.body_digits;
		}, 350);
		return () => clearTimeout(timer);
	});

	async function save(ev: SubmitEvent) {
		ev.preventDefault();
		busy = true;
		try {
			const k = await api.createCustomKind(payload());
			notify.ok(`“${k.name}” tanımlandı`);
			await invalidate('app:data');
			onclose();
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}

	// Henüz hiçbir formül yazılmadıysa hata yerine yönlendirme göster.
	let pristine = $derived(form.rules.every((r) => !r.expr.trim()) && !form.check_expr.trim());
	let example = $derived(fresh && result?.ok ? result.examples?.[0] : undefined);
	let validBodies = $derived(result?.density ? Math.round(result.density * 10 ** resultDigits) : 0);
</script>

<form onsubmit={save}>
	<div class="head">
		<h2>Yeni kod tipi</h2>
		<div class="examples-pick">
			<span class="small muted">Örnek doldur:</span>
			{#each Object.entries(examples) as [key, ex] (key)}
				<button type="button" class="small" onclick={() => fillExample(key as keyof typeof examples)}>
					{ex.label}
				</button>
			{/each}
		</div>
	</div>
	{#if exampleNote}
		<div class="alert warn small" style="margin-bottom: 12px">{exampleNote}</div>
	{/if}

	<div class="row">
		<div class="field">
			<label for="ck-name">Ad</label>
			<input id="ck-name" bind:value={form.name} required placeholder="ör. Ürün kimliği" />
		</div>
		<div class="field">
			<label for="ck-code">Kod</label>
			<input id="ck-code" class="mono" bind:value={form.code} oninput={() => (codeTouched = true)} required />
		</div>
		<div class="field" style="flex: 0 1 120px">
			<label for="ck-body">Gövde (hane)</label>
			<input id="ck-body" type="number" min="1" max="15" bind:value={form.body_digits} />
		</div>
		<div class="field" style="flex: 0 1 120px">
			<label for="ck-check">Kontrol (hane)</label>
			<input id="ck-check" type="number" min="0" max="6" bind:value={form.check_digits} />
		</div>
	</div>

	<!-- Konum cetveli: kurallardaki konum numaraları buna göre. -->
	<div class="ruler" aria-label="Kod yapısı">
		{#each Array.from({ length: form.body_digits }, (_, i) => i) as i (i)}
			<span class="cell"><b>{example ? example[i] : '·'}</b><small>{i + 1}</small></span>
		{/each}
		{#each Array.from({ length: form.check_digits }, (_, i) => i) as i (i)}
			<span class="cell check"><b>{example ? example[form.body_digits + i] : '·'}</b><small>K</small></span>
		{/each}
	</div>

	<h3>Kurallar <span class="muted">(gövdenin her biri doğru olmalı)</span></h3>
	{#each form.rules as rule, i (i)}
		<div class="rule">
			<span class="idx">{i + 1}</span>
			<input class="mono" bind:value={rule.expr} placeholder="ör. digitSum(1, 3) % 2 == 0" />
			<input bind:value={rule.message} placeholder="hata mesajı, ör. ilk 3 hanenin toplamı çift olmalı" />
			<button type="button" class="small" onclick={() => form.rules.splice(i, 1)} aria-label="Kuralı sil">✕</button>
		</div>
	{/each}
	<button type="button" class="small" onclick={() => form.rules.push({ expr: '', message: '' })}>+ Kural ekle</button>

	{#if form.check_digits > 0}
		<div class="field" style="margin-top: 14px">
			<label for="ck-expr">Kontrol formülü <span class="muted">(sonucun son {form.check_digits} hanesi kullanılır)</span></label>
			<input id="ck-expr" class="mono" bind:value={form.check_expr} placeholder="ör. sum(chunks(1, 8, 2))" />
		</div>
	{/if}

	<div class="field">
		<label for="ck-desc">Açıklama</label>
		<input id="ck-desc" bind:value={form.description} placeholder="opsiyonel" />
	</div>

	<details class="help">
		<summary>Kullanılabilir fonksiyonlar</summary>
		<table>
			<tbody>
				<tr><td class="mono">digit(i)</td><td>i. hane (konumlar 1'den başlar)</td></tr>
				<tr><td class="mono">digitSum(a, b)</td><td>a..b hanelerinin toplamı</td></tr>
				<tr><td class="mono">number(a, b)</td><td>a..b hanelerinin oluşturduğu sayı: 12345678 için number(3, 4) = 34</td></tr>
				<tr><td class="mono">chunks(a, b, n)</td><td>a..b hanelerini n haneli sayılara böler: chunks(1, 8, 2) = [12, 34, 56, 78]</td></tr>
				<tr><td class="mono">weightedSum(a, b, [3, 1])</td><td>hanelerin ağırlıklı toplamı, ağırlıklar soldan döner (GS1, Luhn benzeri algoritmalar)</td></tr>
				<tr><td class="mono">sum, map, filter, all, any, count, len, abs, max, min</td><td>liste işlemleri; ör. all(1..8, {'{'}digit(#) != 0{'}'})</td></tr>
				<tr><td class="mono">+ - * % ** == != &lt; &gt; and or not</td><td>işleçler (% kalan, ** üs)</td></tr>
				<tr><td class="mono">x in [1, 3, 5] · a ? b : c · 1..8</td><td>listede var mı · koşullu değer · aralık</td></tr>
				<tr><td class="mono">int(a / b)</td><td>/ ondalıklı sonuç verir; tam bölme için int(...) ya da % kullanın</td></tr>
			</tbody>
		</table>
	</details>

	<div
		class="result"
		class:bad={result && !result.ok && !pristine}
		class:idle={pristine}
		class:stale={!pristine && result && !fresh}
		aria-busy={!pristine && !fresh}
	>
		{#if pristine}
			<span class="muted">
				Kuralları ve kontrol formülünü yazdıkça burada canlı denenir: örnek kodlar, geçerli oran ve hatalar.
			</span>
		{:else if !result}
			<span class="muted">Deneniyor…</span>
		{:else}
			{#if !fresh}<div class="small muted">Deneniyor… (aşağıdaki sonuç önceki tanıma ait)</div>{/if}
			{#if !result.ok}
				✗ {result.error}
			{:else}
				<div>
					✓ Gövdelerin <strong>≈ {pct(result.density ?? 0, 1)}</strong> kadarı kurallara uyuyor →
					{resultDigits} haneli uzayda ≈ <strong>{num(validBodies)}</strong> geçerli kod.
				</div>
				<div class="examples">
					{#each result.examples ?? [] as ex (ex)}
						<span class="mono">{ex.slice(0, resultDigits)}<em>{ex.slice(resultDigits)}</em></span>
					{/each}
				</div>
			{/if}
		{/if}
		<div class="try">
			<input class="mono" placeholder="Bir kodu dene…" bind:value={tryValue} />
			{#if result?.value}
				{#if result.value.valid}
					<span class="ok">✓ geçerli</span>
				{:else}
					<span class="err">✗ {result.value.error}</span>
				{/if}
			{/if}
		</div>
	</div>

	<p class="small muted">
		Kaydettikten sonra tanım değiştirilemez: algoritma değişirse daha önce verilmiş kodlar geçersiz kalır.
	</p>
	<div class="row" style="justify-content: flex-end">
		<button type="button" onclick={onclose}>Vazgeç</button>
		<button class="primary" disabled={busy || !fresh || !result?.ok}>Kaydet</button>
	</div>
</form>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}
	.head h2 {
		margin: 0;
	}
	.examples-pick {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	h3 {
		margin-top: 16px;
	}
	.ruler {
		display: flex;
		gap: 3px;
		flex-wrap: wrap;
		margin: 2px 0 6px;
	}
	.cell {
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		width: 28px;
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 2px 0;
		background: var(--surface-2);
	}
	.cell b {
		font-family: var(--mono);
		font-size: 15px;
	}
	.cell small {
		font-size: 10px;
		color: var(--muted);
	}
	.cell.check {
		background: var(--accent-soft);
		border-color: color-mix(in srgb, var(--accent) 30%, transparent);
	}
	.cell.check small {
		color: var(--accent);
	}
	.rule {
		display: grid;
		grid-template-columns: 22px 1fr 1fr auto;
		gap: 8px;
		align-items: center;
		margin-bottom: 8px;
	}
	.idx {
		color: var(--muted);
		font-size: 12px;
		text-align: right;
	}
	.help {
		margin: 6px 0 14px;
		font-size: 13px;
	}
	.help table {
		margin-top: 8px;
	}
	.help td {
		padding: 4px 8px;
	}
	.result {
		padding: 12px 14px;
		border-radius: var(--radius);
		background: var(--ok-soft);
		margin-bottom: 8px;
	}
	.result.bad {
		background: var(--danger-soft);
		color: var(--danger);
	}
	.result.stale > :not(.try):not(.small) {
		opacity: 0.45;
	}
	.result.idle {
		background: var(--surface-2);
		border: 1px dashed var(--border);
	}
	.examples {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		margin-top: 8px;
	}
	.examples em {
		font-style: normal;
		color: var(--accent);
		font-weight: 600;
	}
	.try {
		display: flex;
		gap: 10px;
		align-items: center;
		margin-top: 10px;
	}
	.try input {
		max-width: 260px;
	}
	.ok {
		color: var(--ok);
	}
	.err {
		color: var(--danger);
	}
</style>
