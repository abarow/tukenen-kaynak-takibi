-- Copyright 2026 Mehmet ERGULCU and the abarow contributors
-- SPDX-License-Identifier: Apache-2.0

-- Kural fonksiyonlarının adları İngilizceye çevrildi:
--   hane → digit, toplam → digitSum, sayi → number, gruplar → chunks, agirlikli → weightedSum
--
-- Kayıtlı özel tiplerde yalnızca formüllerin yazımı güncellenir. Fonksiyonların anlamı
-- aynı kaldığı için algoritma, geçerli oran ve daha önce verilmiş kodlar değişmez.

CREATE FUNCTION kaynak_rename_rule_functions(s text) RETURNS text
    LANGUAGE sql IMMUTABLE AS $$
    SELECT regexp_replace(regexp_replace(regexp_replace(regexp_replace(regexp_replace(s,
        '\mhane\s*\(',      'digit(',       'g'),
        '\mtoplam\s*\(',    'digitSum(',    'g'),
        '\msayi\s*\(',      'number(',      'g'),
        '\mgruplar\s*\(',   'chunks(',      'g'),
        '\magirlikli\s*\(', 'weightedSum(', 'g')
$$;

-- Kural mesajlarına dokunmadan yalnızca her kuralın "expr" alanını çevir.
UPDATE custom_kinds SET
    rules = COALESCE((
        SELECT jsonb_agg(jsonb_set(r, '{expr}', to_jsonb(kaynak_rename_rule_functions(r ->> 'expr'))) ORDER BY ord)
        FROM jsonb_array_elements(rules) WITH ORDINALITY AS e (r, ord)
    ), '[]'::jsonb),
    check_expr = kaynak_rename_rule_functions(check_expr);

DROP FUNCTION kaynak_rename_rule_functions(text);
