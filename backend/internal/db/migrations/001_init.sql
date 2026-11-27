-- Copyright 2026 Mehmet ERGULCU and the abarow contributors
-- SPDX-License-Identifier: Apache-2.0

-- Kaynak yönetimi: temel şema
--
-- Tasarım özeti
--   pools             : Kaynak havuzu (ör. "GS1 barkod aralığımız", "Yayınevi ISBN bloğu").
--   pool_ranges       : Havuza ait sayısal aralıklar. Değerler önceden üretilmez;
--                       next_value imleci "henüz hiç dağıtılmamış" sınırı gösterir.
--   allocations       : Sadece dokunulmuş (rezerve/atanmış/bırakılmış/bloklanmış) değerler
--                       satır olarak tutulur. 1 milyar elemanlı bir aralık bile birkaç satırla başlar.
--   allocation_events : Her durum değişikliğinin değiştirilemez denetim kaydı.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE pools (
    id                      bigserial PRIMARY KEY,
    code                    text        NOT NULL UNIQUE,
    name                    text        NOT NULL,
    description             text        NOT NULL DEFAULT '',
    kind                    text        NOT NULL CHECK (kind IN ('ean13', 'ean8', 'isbn13', 'tckn', 'sequence')),
    prefix                  text        NOT NULL DEFAULT '',
    pad_width               integer     NOT NULL DEFAULT 0 CHECK (pad_width BETWEEN 0 AND 18),
    -- never     : bırakılan değer bir daha kullanılmaz (ISBN, TCKN, GS1 önerisi)
    -- cooldown  : cooldown_days kadar karantinadan sonra yeniden kullanılabilir
    -- immediate : bırakıldığı anda yeniden kullanılabilir
    reuse_policy            text        NOT NULL DEFAULT 'never' CHECK (reuse_policy IN ('never', 'cooldown', 'immediate')),
    cooldown_days           integer     NOT NULL DEFAULT 0 CHECK (cooldown_days >= 0),
    reservation_ttl_seconds integer     NOT NULL DEFAULT 900 CHECK (reservation_ttl_seconds > 0),
    low_watermark           bigint      NOT NULL DEFAULT 0 CHECK (low_watermark >= 0),
    active                  boolean     NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE pool_ranges (
    id          bigserial PRIMARY KEY,
    pool_id     bigint      NOT NULL REFERENCES pools (id) ON DELETE CASCADE,
    -- Çakışma alanı: EAN-13 ve ISBN-13 aynı GTIN-13 uzayını paylaşır, bu yüzden
    -- ikisi de 'gtin13' olur. Sıra numaralarında önek alanı ayırır ('seq:FTR-').
    namespace   text        NOT NULL,
    start_value bigint      NOT NULL,
    end_value   bigint      NOT NULL,
    next_value  bigint      NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (start_value >= 0 AND start_value <= end_value),
    CHECK (next_value BETWEEN start_value AND end_value + 1),
    -- Aynı uzayda iki aralık (farklı havuzlarda bile) asla çakışamaz.
    CONSTRAINT pool_ranges_no_overlap EXCLUDE USING gist (
        namespace WITH =,
        int8range(start_value, end_value, '[]') WITH &&
    )
);

CREATE INDEX pool_ranges_pool ON pool_ranges (pool_id, start_value);

CREATE TABLE allocations (
    id             bigserial PRIMARY KEY,
    pool_id        bigint      NOT NULL REFERENCES pools (id),
    range_id       bigint      NOT NULL REFERENCES pool_ranges (id),
    base_value     bigint      NOT NULL,
    value          text        NOT NULL,
    status         text        NOT NULL CHECK (status IN ('reserved', 'assigned', 'released', 'blocked')),
    object_type    text,
    object_ref     text,
    note           text        NOT NULL DEFAULT '',
    metadata       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    reserved_until timestamptz,
    -- NULL değilse ve geçmişteyse, 'released' değer yeniden dağıtılabilir.
    reusable_at    timestamptz,
    assigned_at    timestamptz,
    released_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (pool_id, base_value),
    CHECK (status NOT IN ('reserved', 'assigned') OR (object_type IS NOT NULL AND object_ref IS NOT NULL)),
    CHECK (status <> 'reserved' OR reserved_until IS NOT NULL)
);

-- Bir nesnenin aynı havuzdan aynı anda tek bir aktif değeri olabilir (idempotent tahsis).
CREATE UNIQUE INDEX allocations_active_object
    ON allocations (pool_id, object_type, object_ref)
    WHERE status IN ('reserved', 'assigned');

CREATE INDEX allocations_reusable ON allocations (pool_id, reusable_at, base_value)
    WHERE status = 'released' AND reusable_at IS NOT NULL;
CREATE INDEX allocations_expiry ON allocations (reserved_until) WHERE status = 'reserved';
CREATE INDEX allocations_value ON allocations (value);
CREATE INDEX allocations_object ON allocations (object_type, object_ref);
CREATE INDEX allocations_pool_status ON allocations (pool_id, status, updated_at DESC);

CREATE TABLE allocation_events (
    id            bigserial PRIMARY KEY,
    allocation_id bigint      NOT NULL REFERENCES allocations (id),
    pool_id       bigint      NOT NULL REFERENCES pools (id),
    value         text        NOT NULL,
    action        text        NOT NULL,
    from_status   text,
    to_status     text        NOT NULL,
    object_type   text,
    object_ref    text,
    actor         text        NOT NULL DEFAULT '',
    note          text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX allocation_events_allocation ON allocation_events (allocation_id, id DESC);
CREATE INDEX allocation_events_pool ON allocation_events (pool_id, id DESC);
