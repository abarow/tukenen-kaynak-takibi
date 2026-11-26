// adapter-static çıktı klasörünü temizlerken .gitkeep'i de siler.
// Go'nun //go:embed all:dist yönergesi klasörün var olmasını istediği için geri koyuyoruz.
import { writeFileSync } from 'node:fs';

writeFileSync(new URL('../../backend/internal/web/dist/.gitkeep', import.meta.url), '');
