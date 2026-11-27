#!/usr/bin/env bash
# Copyright 2026 Mehmet ERGULCU and the abarow contributors
# SPDX-License-Identifier: Apache-2.0

# check-headers.sh, depodaki her kaynak dosyanın telif başlığını taşıdığını denetler:
# kurucu satırı ("Copyright <yıl> Mehmet ERGULCU and the abarow contributors") ve SPDX satırı
# ilk 10 satırda olmalıdır. Katkı verenler kendi satırlarını ekleyebilir; kurucu satırı
# silinemez (bkz. CONTRIBUTING.md). Henüz commit edilmemiş yeni dosyalar da denetlenir.
set -euo pipefail
cd "$(dirname "$0")/.."

founder='Copyright [0-9]{4} Mehmet ERGULCU and the abarow contributors'
spdx='SPDX-License-Identifier: Apache-2.0'

missing=0
while IFS= read -r f; do
	[ -f "$f" ] || continue
	top=$(head -n 10 "$f")
	if ! grep -Eq "$founder" <<<"$top" || ! grep -Fq "$spdx" <<<"$top"; then
		echo "telif başlığı eksik: $f"
		missing=$((missing + 1))
	fi
done < <(git ls-files --cached --others --exclude-standard '*.go' '*.sql' '*.sh' '*.js' '*.mjs' '*.ts' '*.svelte' '*.css' '*.html' '*.yml' '*.yaml' 'Dockerfile' '*/Dockerfile')

if [ "$missing" -gt 0 ]; then
	echo "$missing dosyada telif başlığı eksik; biçim için CONTRIBUTING.md'ye bakın."
	exit 1
fi
echo "telif başlıkları tamam"
