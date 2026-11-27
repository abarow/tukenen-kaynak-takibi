#!/usr/bin/env node
// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

// third-party-notices.mjs, sunucu ikili dosyasına ve arayüze giren üçüncü taraf kodun lisanslarını
// THIRD_PARTY_NOTICES dosyasına yazar ve dosyanın güncel olduğunu denetler.
//
//   node scripts/third-party-notices.mjs           # THIRD_PARTY_NOTICES dosyasını yeniden yazar
//   node scripts/third-party-notices.mjs --check   # dosya eskiyse ya da bir bağımlılığın lisansı
//                                                  # karar gerektiriyorsa 1 ile çıkar
//
// Kapsam:
//   - sunucunun derlendiği Go modülleri (go list -deps ./cmd/server) ve Go standart kütüphanesi;
//     lisans metinleri modül önbelleğinden okunur;
//   - Vite derlemesinin arayüz dosyalarına koyduğu npm paketleri (UI_PACKAGES).
// Go modül önbelleği ve frontend/node_modules (npm ci) gerekir.

import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUT = join(ROOT, 'THIRD_PARTY_NOTICES');
// Derlemenin arayüze koyduğu paketler: SvelteKit'in istemci kodu ve onun kullandığı devalue,
// Svelte'in çalışma zamanı ve onun paketlediği clsx, Vite'ın kendi yardımcı kodu (modül ön
// yükleme). Liste, kaynak haritalı bir derlemenin (npx vite build --sourcemap) haritalarındaki
// node_modules yollarından çıkarıldı. Diğer bağımlılıklar yalnızca derleme sırasında çalışır.
const UI_PACKAGES = ['@sveltejs/kit', 'clsx', 'devalue', 'svelte', 'vite'];
const ALLOWED = new Set(['Apache-2.0', 'MIT', 'BSD-2-Clause', 'BSD-3-Clause', 'ISC']);
const LINE = '-'.repeat(79);

const run = (cmd, args, cwd) => execFileSync(cmd, args, { cwd, encoding: 'utf8' }).trim();
const read = (p) => readFileSync(p, 'utf8').replace(/\r\n/g, '\n').replace(/[ \t]+$/gm, '').trim();

function kind(text) {
	const t = text.slice(0, 6000);
	if (/Apache License/.test(t) && /Version 2\.0/.test(t)) return 'Apache-2.0';
	if (/Mozilla Public License/.test(t)) return 'MPL';
	if (/GNU (LESSER|LIBRARY) GENERAL PUBLIC/i.test(t)) return 'LGPL';
	if (/GNU AFFERO/i.test(t)) return 'AGPL';
	if (/GNU GENERAL PUBLIC/i.test(t)) return 'GPL';
	if (/Permission is hereby granted, free of charge/.test(t)) return 'MIT';
	if (/Permission to use, copy, modify, and\/or distribute/.test(t)) return 'ISC';
	if (/Redistribution and use in source and binary forms/.test(t)) return /Neither the name/.test(t) ? 'BSD-3-Clause' : 'BSD-2-Clause';
	return 'bilinmiyor';
}

function licenseFiles(dir) {
	return readdirSync(dir)
		.filter((f) => /^(LICEN[CS]E|COPYING|NOTICE|PATENTS)([.-].*)?$/i.test(f) && !/\.(go|js|ts|json)$/.test(f))
		.sort()
		// Vite'ın LICENSE.md'sinin geri kalanı, Vite'ın kendi içine paketlediği (arayüze girmeyen)
		// paketlerin lisanslarıdır.
		.map((f) => ({ file: f, text: read(join(dir, f)).split('# Licenses of bundled dependencies')[0].trim() }));
}

function goModules() {
	const backend = join(ROOT, 'backend');
	const out = run('go', ['list', '-deps', '-f', '{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}{{end}}', './cmd/server'], backend);
	const mods = new Map();
	for (const line of out.split('\n').filter(Boolean)) {
		const [path, version, dir] = line.split('\t');
		mods.set(path, { eco: 'go', name: path, version, dir });
	}
	return [...mods.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function goVersion() {
	const mod = readFileSync(join(ROOT, 'backend', 'go.mod'), 'utf8');
	const m = mod.match(/^go (\d+\.\d+)/m);
	return m ? `Go ${m[1]}` : 'Go';
}

function npmPackages() {
	return UI_PACKAGES.map((name) => {
		const dir = join(ROOT, 'frontend', 'node_modules', ...name.split('/'));
		if (!existsSync(dir)) throw new Error(`${name} kurulu değil: frontend klasöründe npm ci çalıştırın`);
		const pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'));
		return { eco: 'npm', name, version: pkg.version, dir, declared: pkg.license };
	});
}

function build() {
	const std = { eco: 'go', name: 'Go standart kütüphanesi / Go standard library', version: '', dir: run('go', ['env', 'GOROOT']) };
	const all = [std, ...goModules(), ...npmPackages()];
	const problems = [];
	for (const c of all) {
		c.files = licenseFiles(c.dir);
		const main = c.files.find((f) => !/^(NOTICE|PATENTS)/i.test(f.file));
		c.license = main ? kind(main.text) : c.declared ?? 'bilinmiyor';
		if (!main) problems.push(`${c.name}: lisans dosyası bulunamadı`);
		else if (!ALLOWED.has(c.license)) problems.push(`${c.name}: ${c.license} lisansı karar gerektirir (CONTRIBUTING.md)`);
	}
	const label = (c) => (c.version ? `${c.name} ${c.version}` : c.name);
	const [goStd, ...rest] = all;
	const gos = rest.filter((c) => c.eco === 'go');
	const npms = rest.filter((c) => c.eco === 'npm');

	let s = `abarow · Tükenen Kaynak Takibi: THIRD-PARTY NOTICES

Tükenen Kaynak Takibi is under the Apache License 2.0 (LICENSE; NOTICE). The server binary and the
interface it embeds also hold the code of the projects below, each under its own license. This file
reproduces their notices. It is made by scripts/third-party-notices.mjs and ships in the image as
/licenses/THIRD_PARTY_NOTICES.

${LINE}
INVENTORY (checked by scripts/third-party-notices.mjs --check)

${rest.map((c) => `  ${c.eco} ${c.name} ${c.version}`).join('\n')}

${LINE}

1. GO STANDARD LIBRARY AND MODULES

The server is a static Go program built with ${goVersion()}; the standard library
(${goStd.license}, The Go Authors) and these modules are inside it.

Modules:
${gos.map((c) => `  ${label(c)}  [${c.license}]`).join('\n')}

2. NPM PACKAGES IN THE INTERFACE

The interface is built with SvelteKit and Vite; the build puts code from these packages into the
files the server embeds.

Packages:
${npms.map((c) => `  ${label(c)}  [${c.license}]`).join('\n')}

3. LICENSE TEXTS
`;
	// Aynı metni taşıyan bileşenler tek metin altında toplanır.
	const groups = new Map();
	for (const c of all) {
		for (const f of c.files) {
			if (!groups.has(f.text)) groups.set(f.text, []);
			groups.get(f.text).push(`${label(c)} (${f.file})`);
		}
	}
	for (const [text, who] of groups) {
		s += `\n${LINE}\nApplies to:\n${who.map((w) => `  - ${w}`).join('\n')}\n${LINE}\n${text}\n`;
	}
	return { text: s, problems };
}

const { text, problems } = build();
if (process.argv.includes('--check')) {
	const current = existsSync(OUT) ? readFileSync(OUT, 'utf8').replace(/\r\n/g, '\n') : '';
	let ok = true;
	if (current !== text) { console.error('THIRD_PARTY_NOTICES güncel değil: node scripts/third-party-notices.mjs'); ok = false; }
	for (const p of problems) { console.error(p); ok = false; }
	if (!ok) process.exit(1);
	console.log('THIRD_PARTY_NOTICES güncel');
} else {
	writeFileSync(OUT, text);
	for (const p of problems) console.error('uyarı: ' + p);
	console.log(`THIRD_PARTY_NOTICES yazıldı (${text.split('\n').length} satır)`);
}
