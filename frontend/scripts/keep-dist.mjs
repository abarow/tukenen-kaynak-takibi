// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

// adapter-static çıktı klasörünü temizlerken .gitkeep'i de siler.
// Go'nun //go:embed all:dist yönergesi klasörün var olmasını istediği için geri koyuyoruz.
import { writeFileSync } from 'node:fs';

writeFileSync(new URL('../../backend/internal/web/dist/.gitkeep', import.meta.url), '');
