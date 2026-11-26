-- IMEI tipi kaldırıldı; yerine MAC adresi (EUI-48: onaltılık, I/G ve U/L bit kuralları) eklendi.
-- 48 bitlik adres bigint'e sığar; pool_ranges ve allocations şeması değişmez.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pools WHERE kind = 'imei') THEN
        RAISE EXCEPTION 'imei tipinde havuz var; bu migration öncesinde taşınmalı veya silinmeli';
    END IF;
END $$;

ALTER TABLE pools DROP CONSTRAINT pools_kind_check;
ALTER TABLE pools ADD CONSTRAINT pools_kind_check
    CHECK (kind IN ('ean13', 'ean8', 'isbn13', 'mac', 'sequence'));
