-- Copyright 2026 Mehmet ERGULCU and the abarow contributors
-- SPDX-License-Identifier: Apache-2.0

-- TCKN tipi kaldırıldı; yerine IMEI (mobil cihaz kimliği, Luhn kontrol hanesi) eklendi.
--
-- 001 zaten uygulanmış veritabanları için CHECK kısıtını günceller. TCKN tipinde
-- havuz varsa sessizce bozmak yerine açık bir hatayla durur.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pools WHERE kind = 'tckn') THEN
        RAISE EXCEPTION 'tckn tipinde havuz var; bu migration öncesinde taşınmalı veya silinmeli';
    END IF;
END $$;

ALTER TABLE pools DROP CONSTRAINT pools_kind_check;
ALTER TABLE pools ADD CONSTRAINT pools_kind_check
    CHECK (kind IN ('ean13', 'ean8', 'isbn13', 'imei', 'sequence'));
