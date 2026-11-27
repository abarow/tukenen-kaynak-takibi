// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Derleme çıktısı doğrudan Go sunucusunun gömdüğü klasöre yazılır;
// böylece tek bir ikili dosya hem API'yi hem arayüzü sunar.
const out = '../backend/internal/web/dist';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({ pages: out, assets: out, fallback: '200.html' })
		})
	],
	server: {
		proxy: {
			'/api': process.env.API_URL ?? 'http://localhost:8088'
		}
	}
});
