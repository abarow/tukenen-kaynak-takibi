-- Copyright 2026 Mehmet ERGULCU and the abarow contributors
-- SPDX-License-Identifier: Apache-2.0

-- Kullanıcı tanımlı kod tipleri (ör. kurallı ürün kimliği).
--
-- Tanım oluşturulduktan sonra değişmez: algoritma değişirse daha önce verilmiş
-- kodların geçerliliği bozulur. Düzeltmek için kullanılmayan tip silinip yeniden tanımlanır.

CREATE TABLE custom_kinds (
    id           bigserial PRIMARY KEY,
    code         text        NOT NULL UNIQUE,
    name         text        NOT NULL,
    description  text        NOT NULL DEFAULT '',
    body_digits  integer     NOT NULL CHECK (body_digits BETWEEN 1 AND 15),
    check_digits integer     NOT NULL CHECK (check_digits BETWEEN 0 AND 6),
    rules        jsonb       NOT NULL DEFAULT '[]'::jsonb,
    check_expr   text        NOT NULL DEFAULT '',
    density      double precision NOT NULL,   -- kurallara uyan gövdelerin örneklemle ölçülen oranı
    example      text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (body_digits + check_digits <= 18)
);

ALTER TABLE pools ADD COLUMN custom_kind_id bigint REFERENCES custom_kinds (id);
ALTER TABLE pools DROP CONSTRAINT pools_kind_check;
ALTER TABLE pools ADD CONSTRAINT pools_kind_check
    CHECK (kind IN ('ean13', 'ean8', 'isbn13', 'mac', 'sequence', 'custom'));
ALTER TABLE pools ADD CONSTRAINT pools_custom_kind_check
    CHECK ((kind = 'custom') = (custom_kind_id IS NOT NULL));

-- Kurallı tiplerde aralıktaki her gövde geçerli değildir. Kapasite için geçerli
-- değer sayısı aralık eklenirken hesaplanır (büyük aralıklarda örneklem tahmini).
-- NULL: aralıktaki her gövde geçerli (yerleşik tipler).
ALTER TABLE pool_ranges ADD COLUMN valid_count bigint CHECK (valid_count >= 0);
ALTER TABLE pool_ranges ADD COLUMN valid_count_exact boolean NOT NULL DEFAULT true;
