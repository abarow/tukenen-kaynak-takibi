package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ModeAssign  = "assign"
	ModeReserve = "reserve"
)

const allocColumns = `a.id, a.pool_id, p.code, p.name, p.kind, COALESCE(ck.name, ''), a.value, a.status, a.object_type, a.object_ref,
	a.note, a.metadata, a.reserved_until, a.reusable_at, a.assigned_at, a.released_at, a.created_at, a.updated_at`

const allocFrom = ` FROM allocations a JOIN pools p ON p.id = a.pool_id LEFT JOIN custom_kinds ck ON ck.id = p.custom_kind_id`

func scanAllocation(row pgx.Row) (*Allocation, error) {
	var a Allocation
	var meta []byte
	err := row.Scan(&a.ID, &a.PoolID, &a.PoolCode, &a.PoolName, &a.Kind, &a.KindName, &a.Value, &a.Status, &a.ObjectType, &a.ObjectRef,
		&a.Note, &meta, &a.ReservedUntil, &a.ReusableAt, &a.AssignedAt, &a.ReleasedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Metadata = json.RawMessage(meta)
	return &a, nil
}

func getAllocationTx(ctx context.Context, tx pgx.Tx, id int64) (*Allocation, error) {
	return scanAllocation(tx.QueryRow(ctx, `SELECT `+allocColumns+allocFrom+` WHERE a.id = $1`, id))
}

// ---- Tahsis ---------------------------------------------------------------

// AllocateRequest bir nesne için havuzdan değer ister.
//
// Aynı (havuz, object_type, object_ref) için tekrar çağrılırsa yeni değer
// üretilmez, mevcut aktif tahsis döner. Bu sayede istemci ağ hatasında
// güvenle tekrar deneyebilir.
type AllocateRequest struct {
	ObjectType string          `json:"object_type"`
	ObjectRef  string          `json:"object_ref"`
	Mode       string          `json:"mode"`            // assign (varsayılan) | reserve
	Value      string          `json:"value,omitempty"` // belirli bir değer isteniyorsa
	TTLSeconds int             `json:"ttl_seconds,omitempty"`
	Note       string          `json:"note,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

func (r *AllocateRequest) normalize() error {
	r.ObjectType = strings.TrimSpace(r.ObjectType)
	r.ObjectRef = strings.TrimSpace(r.ObjectRef)
	r.Value = strings.TrimSpace(r.Value)
	if r.ObjectType == "" || r.ObjectRef == "" {
		return invalidf("nesne tipi ve nesne referansı zorunlu")
	}
	if len(r.ObjectType) > 64 || len(r.ObjectRef) > 256 {
		return invalidf("nesne tipi en fazla 64, referans en fazla 256 karakter olabilir")
	}
	if r.Mode == "" {
		r.Mode = ModeAssign
	}
	if r.Mode != ModeAssign && r.Mode != ModeReserve {
		return invalidf("mode assign veya reserve olmalı")
	}
	if r.TTLSeconds < 0 || r.TTLSeconds > 30*24*3600 {
		return invalidf("ttl_seconds 0 ile 30 gün arasında olmalı")
	}
	if len(r.Note) > 2000 {
		return invalidf("not en fazla 2000 karakter olabilir")
	}
	if len(r.Metadata) == 0 || string(r.Metadata) == "null" {
		r.Metadata = json.RawMessage(`{}`)
	}
	var obj map[string]any
	if err := json.Unmarshal(r.Metadata, &obj); err != nil {
		return invalidf("metadata bir JSON nesnesi olmalı")
	}
	return nil
}

func (s *Store) Allocate(ctx context.Context, poolID int64, req AllocateRequest, actor string) (*Allocation, bool, error) {
	if err := req.normalize(); err != nil {
		return nil, false, err
	}
	var out *Allocation
	var created bool
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		p, err := activePoolTx(ctx, tx, poolID)
		if err != nil {
			return err
		}
		out, created, err = allocateTx(ctx, tx, p, req, actor)
		return err
	})
	return out, created, err
}

type BulkResult struct {
	Allocation *Allocation `json:"allocation"`
	Created    bool        `json:"created"`
}

// AllocateBulk tüm istekleri tek transaction'da işler: biri başarısız olursa hiçbiri yazılmaz.
func (s *Store) AllocateBulk(ctx context.Context, poolID int64, reqs []AllocateRequest, actor string) ([]BulkResult, error) {
	if len(reqs) == 0 || len(reqs) > 1000 {
		return nil, invalidf("toplu istekte 1-1000 kalem olmalı")
	}
	for i := range reqs {
		if err := reqs[i].normalize(); err != nil {
			return nil, invalidf("%d. kalem: %s", i+1, err.Error())
		}
	}
	out := make([]BulkResult, 0, len(reqs))
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		p, err := activePoolTx(ctx, tx, poolID)
		if err != nil {
			return err
		}
		for i, r := range reqs {
			a, created, err := allocateTx(ctx, tx, p, r, actor)
			if err != nil {
				var e *Error
				if errors.As(err, &e) {
					return &Error{e.Kind, e.Code, fmt.Sprintf("%d. kalem (%s/%s): %s", i+1, r.ObjectType, r.ObjectRef, e.Message)}
				}
				return err
			}
			out = append(out, BulkResult{a, created})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func activePoolTx(ctx context.Context, tx pgx.Tx, poolID int64) (*Pool, error) {
	p, err := getPoolTx(ctx, tx, poolID)
	if err != nil {
		return nil, err
	}
	if !p.Active {
		return nil, conflictf("pool_inactive", "%q havuzu pasif; tahsis yapılamaz", p.Code)
	}
	return p, nil
}

func allocateTx(ctx context.Context, tx pgx.Tx, p *Pool, req AllocateRequest, actor string) (*Allocation, bool, error) {
	codec := p.Codec()
	ttl := req.TTLSeconds
	if ttl == 0 {
		ttl = p.ReservationTTLSeconds
	}
	status := StatusAssigned
	if req.Mode == ModeReserve {
		status = StatusReserved
	}

	wantBase := int64(-1)
	if req.Value != "" {
		b, err := codec.Parse(req.Value)
		if err != nil {
			return nil, false, fromKindsErr(err)
		}
		wantBase = b
	}

	// Aynı nesne için eşzamanlı gelen istekleri sıraya sok. Kilit transaction
	// sonunda kendiliğinden bırakılır.
	lockKey := fmt.Sprintf("alloc|%d|%s|%s", p.ID, req.ObjectType, req.ObjectRef)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return nil, false, err
	}

	// 1) Nesnenin bu havuzda zaten aktif bir değeri var mı? (idempotency)
	existing, err := scanAllocation(tx.QueryRow(ctx, `SELECT `+allocColumns+allocFrom+`
		WHERE a.pool_id = $1 AND a.object_type = $2 AND a.object_ref = $3 AND a.status IN ('reserved', 'assigned')
		FOR UPDATE OF a`, p.ID, req.ObjectType, req.ObjectRef))
	switch {
	case err == nil:
		if wantBase >= 0 && codec.Format(wantBase) != existing.Value {
			return nil, false, conflictf("object_has_value", "bu nesneye %q havuzundan zaten %s verilmiş", p.Code, existing.Value)
		}
		if existing.Status != StatusReserved {
			return existing, false, nil
		}
		// Rezervasyon var: assign isteniyorsa onayla, reserve isteniyorsa süreyi uzat.
		action := "renew"
		if req.Mode == ModeAssign {
			action = "confirm"
			_, err = tx.Exec(ctx, `UPDATE allocations SET status = 'assigned', assigned_at = now(), reserved_until = NULL,
				updated_at = now() WHERE id = $1`, existing.ID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE allocations SET reserved_until = now() + $2::int * interval '1 second',
				updated_at = now() WHERE id = $1`, existing.ID, ttl)
		}
		if err != nil {
			return nil, false, err
		}
		a, err := getAllocationTx(ctx, tx, existing.ID)
		if err != nil {
			return nil, false, err
		}
		return a, false, insertEvent(ctx, tx, a, action, StatusReserved, actor, req.Note)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, false, err
	}

	// 2) Değer bul: (a) istenen belirli değer, (b) yeniden kullanılabilir değer, (c) imleçten yeni değer.
	var slotID, rangeID, id int64
	if wantBase >= 0 {
		err := tx.QueryRow(ctx, `SELECT id FROM pool_ranges WHERE pool_id = $1 AND $2 BETWEEN start_value AND end_value`,
			p.ID, wantBase).Scan(&rangeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, invalidf("%s bu havuzun aralıklarının dışında", codec.Format(wantBase))
		}
		if err != nil {
			return nil, false, err
		}
		var cur string
		var reusable bool
		err = tx.QueryRow(ctx, `SELECT id, status, (status = 'released' AND reusable_at IS NOT NULL AND reusable_at <= now())
			FROM allocations WHERE pool_id = $1 AND base_value = $2 FOR UPDATE`, p.ID, wantBase).Scan(&slotID, &cur, &reusable)
		switch {
		case err == nil && !reusable:
			return nil, false, conflictf("value_taken", "%s kullanılamaz (durum: %s)", codec.Format(wantBase), cur)
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return nil, false, err
		}
	} else {
		// SKIP LOCKED: başka bir transaction'ın o an aldığı satırı beklemeden sıradakine geç.
		err := tx.QueryRow(ctx, `SELECT id FROM allocations
			WHERE pool_id = $1 AND status = 'released' AND reusable_at <= now()
			ORDER BY reusable_at, base_value LIMIT 1 FOR UPDATE SKIP LOCKED`, p.ID).Scan(&slotID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
	}

	fromStatus := ""
	switch {
	case slotID != 0:
		fromStatus = StatusReleased
		err = tx.QueryRow(ctx, `UPDATE allocations SET status = $2::text, object_type = $3, object_ref = $4, note = $5,
				metadata = $6::jsonb,
				reserved_until = CASE WHEN $2::text = 'reserved' THEN now() + $7::int * interval '1 second' END,
				assigned_at = CASE WHEN $2::text = 'assigned' THEN now() END,
				released_at = NULL, reusable_at = NULL, updated_at = now()
			WHERE id = $1 RETURNING id`,
			slotID, status, req.ObjectType, req.ObjectRef, req.Note, string(req.Metadata), ttl).Scan(&id)
		if err != nil {
			return nil, false, err
		}
	case wantBase >= 0:
		id, err = insertAllocation(ctx, tx, p, rangeID, wantBase, codec.Format(wantBase), status, req, ttl)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, conflictf("value_taken", "%s az önce başka bir işlemle alındı", codec.Format(wantBase))
		}
		if err != nil {
			return nil, false, err
		}
	default:
		if id, err = allocateFromCursor(ctx, tx, p, status, req, ttl); err != nil {
			return nil, false, err
		}
	}

	a, err := getAllocationTx(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	action := "assign"
	if status == StatusReserved {
		action = "reserve"
	}
	return a, true, insertEvent(ctx, tx, a, action, fromStatus, actor, req.Note)
}

// cursorScanLimit bir tahsiste kurallara uyan değer ararken taranacak en fazla gövde (~0,5 sn).
const cursorScanLimit = 2_000_000

// allocateFromCursor aralık imlecini bir ilerletip o değeri tahsis eder.
// Aralık satırı FOR UPDATE ile kilitlendiği için aynı değer iki kez verilemez;
// transaction geri alınırsa imleç de geri alınır (boşluk oluşmaz).
func allocateFromCursor(ctx context.Context, tx pgx.Tx, p *Pool, status string, req AllocateRequest, ttl int) (int64, error) {
	codec := p.Codec()
	for {
		var rangeID, next, end int64
		err := tx.QueryRow(ctx, `SELECT id, next_value, end_value FROM pool_ranges
			WHERE pool_id = $1 AND next_value <= end_value
			ORDER BY start_value LIMIT 1 FOR UPDATE`, p.ID).Scan(&rangeID, &next, &end)
		if errors.Is(err, pgx.ErrNoRows) {
			// FOR UPDATE + LIMIT: beklediğimiz satır o arada dolduysa sorgu boş döner,
			// ama başka aralıkta yer olabilir. Yeni anlık görüntüyle tekrar bak.
			var more bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pool_ranges WHERE pool_id = $1 AND next_value <= end_value)`,
				p.ID).Scan(&more); err != nil {
				return 0, err
			}
			if more {
				continue
			}
			return 0, conflictf("pool_exhausted", "%q havuzunda boş değer kalmadı", p.Code)
		}
		if err != nil {
			return 0, err
		}
		// Kurallı tiplerde imleç, kurallara uymayan gövdeleri atlar (aralık kilitliyken bellekte tarar).
		found, ok, err := codec.FirstValid(next, end, cursorScanLimit)
		if err != nil {
			return 0, fromKindsErr(err)
		}
		if !ok {
			// Aralığın kalanında geçerli değer yok: aralığı tükenmiş işaretle, sıradakine geç.
			if _, err := tx.Exec(ctx, `UPDATE pool_ranges SET next_value = end_value + 1 WHERE id = $1`, rangeID); err != nil {
				return 0, err
			}
			continue
		}
		next = found
		if _, err := tx.Exec(ctx, `UPDATE pool_ranges SET next_value = $2::bigint + 1 WHERE id = $1`, rangeID, next); err != nil {
			return 0, err
		}
		id, err := insertAllocation(ctx, tx, p, rangeID, next, codec.Format(next), status, req, ttl)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // bu değer daha önce elle alınmış veya bloklanmış; atla
		}
		return id, err
	}
}

func insertAllocation(ctx context.Context, tx pgx.Tx, p *Pool, rangeID, base int64, value, status string, req AllocateRequest, ttl int) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO allocations
			(pool_id, range_id, base_value, value, status, object_type, object_ref, note, metadata, reserved_until, assigned_at)
		VALUES ($1, $2, $3, $4, $5::text, $6, $7, $8, $9::jsonb,
			CASE WHEN $5::text = 'reserved' THEN now() + $10::int * interval '1 second' END,
			CASE WHEN $5::text = 'assigned' THEN now() END)
		ON CONFLICT (pool_id, base_value) DO NOTHING
		RETURNING id`,
		p.ID, rangeID, base, value, status, req.ObjectType, req.ObjectRef, req.Note, string(req.Metadata), ttl).Scan(&id)
	return id, err
}

func insertEvent(ctx context.Context, tx pgx.Tx, a *Allocation, action, fromStatus, actor, note string) error {
	var from *string
	if fromStatus != "" {
		from = &fromStatus
	}
	_, err := tx.Exec(ctx, `INSERT INTO allocation_events
		(allocation_id, pool_id, value, action, from_status, to_status, object_type, object_ref, actor, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.ID, a.PoolID, a.Value, action, from, a.Status, a.ObjectType, a.ObjectRef, actor, note)
	return err
}

// ---- Durum geçişleri --------------------------------------------------------

// Confirm rezervasyonu kalıcı atamaya çevirir.
func (s *Store) Confirm(ctx context.Context, id int64, note, actor string) (*Allocation, error) {
	var out *Allocation
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var status string
		var expired bool
		err := tx.QueryRow(ctx, `SELECT status, COALESCE(reserved_until <= now(), false) FROM allocations WHERE id = $1 FOR UPDATE`, id).
			Scan(&status, &expired)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFoundf("tahsis bulunamadı")
		}
		if err != nil {
			return err
		}
		if status != StatusReserved {
			return conflictf("invalid_state", "yalnızca rezerve değerler onaylanabilir (mevcut durum: %s)", status)
		}
		if expired {
			return conflictf("reservation_expired", "rezervasyon süresi dolmuş; yeniden tahsis isteyin")
		}
		if _, err := tx.Exec(ctx, `UPDATE allocations SET status = 'assigned', assigned_at = now(), reserved_until = NULL,
			updated_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		if out, err = getAllocationTx(ctx, tx, id); err != nil {
			return err
		}
		return insertEvent(ctx, tx, out, "confirm", StatusReserved, actor, note)
	})
	return out, err
}

// Release değeri nesneden ayırır. Ne zaman tekrar dağıtılabileceği havuz politikasına bağlıdır:
// hiç kullanılmamış (rezerve/bloklu) değerler hemen, atanmış değerler politikaya göre.
func (s *Store) Release(ctx context.Context, id int64, note, actor string) (*Allocation, error) {
	var out *Allocation
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var status, policy string
		var cooldown int
		err := tx.QueryRow(ctx, `SELECT a.status, p.reuse_policy, p.cooldown_days
			FROM allocations a JOIN pools p ON p.id = a.pool_id WHERE a.id = $1 FOR UPDATE OF a`, id).
			Scan(&status, &policy, &cooldown)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFoundf("tahsis bulunamadı")
		}
		if err != nil {
			return err
		}
		reuse := "now"
		action := "release"
		switch status {
		case StatusReleased:
			return conflictf("invalid_state", "değer zaten bırakılmış")
		case StatusBlocked:
			action = "unblock"
		case StatusAssigned:
			switch policy {
			case ReuseNever:
				reuse = "never"
			case ReuseCooldown:
				reuse = "cooldown"
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE allocations SET status = 'released', released_at = now(), reserved_until = NULL,
				reusable_at = CASE $2::text WHEN 'now' THEN now() WHEN 'never' THEN NULL
					ELSE now() + $3::int * interval '1 day' END,
				updated_at = now()
			WHERE id = $1`, id, reuse, cooldown); err != nil {
			return err
		}
		if out, err = getAllocationTx(ctx, tx, id); err != nil {
			return err
		}
		return insertEvent(ctx, tx, out, action, status, actor, note)
	})
	return out, err
}

// Block bir değeri dağıtım dışı bırakır (ör. başka sistemde kullanılmış, hasarlı etiket, özel amaç).
func (s *Store) Block(ctx context.Context, poolID int64, value, note, actor string) (*Allocation, error) {
	var out *Allocation
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		p, err := getPoolTx(ctx, tx, poolID)
		if err != nil {
			return err
		}
		codec := p.Codec()
		base, err := codec.Parse(value)
		if err != nil {
			return fromKindsErr(err)
		}
		var rangeID int64
		err = tx.QueryRow(ctx, `SELECT id FROM pool_ranges WHERE pool_id = $1 AND $2 BETWEEN start_value AND end_value`,
			p.ID, base).Scan(&rangeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidf("%s bu havuzun aralıklarının dışında", codec.Format(base))
		}
		if err != nil {
			return err
		}

		var id int64
		var cur string
		err = tx.QueryRow(ctx, `SELECT id, status FROM allocations WHERE pool_id = $1 AND base_value = $2 FOR UPDATE`,
			p.ID, base).Scan(&id, &cur)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			err = tx.QueryRow(ctx, `INSERT INTO allocations (pool_id, range_id, base_value, value, status, note)
				VALUES ($1, $2, $3, $4, 'blocked', $5) ON CONFLICT (pool_id, base_value) DO NOTHING RETURNING id`,
				p.ID, rangeID, base, codec.Format(base), note).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return conflictf("value_taken", "%s az önce başka bir işlemle alındı", codec.Format(base))
			}
			cur = ""
		case err != nil:
			return err
		case cur == StatusBlocked:
			return conflictf("invalid_state", "%s zaten bloklu", codec.Format(base))
		case cur != StatusReleased:
			return conflictf("value_taken", "%s şu an kullanımda (durum: %s); önce bırakın", codec.Format(base), cur)
		default:
			_, err = tx.Exec(ctx, `UPDATE allocations SET status = 'blocked', reusable_at = NULL, note = $2,
				updated_at = now() WHERE id = $1`, id, note)
		}
		if err != nil {
			return err
		}
		if out, err = getAllocationTx(ctx, tx, id); err != nil {
			return err
		}
		return insertEvent(ctx, tx, out, "block", cur, actor, note)
	})
	return out, err
}

// ExpireReservations süresi dolan rezervasyonları boşa düşürür. Hiç kullanılmamış
// oldukları için havuz politikasından bağımsız olarak hemen yeniden dağıtılabilirler.
func (s *Store) ExpireReservations(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		WITH expired AS (
			UPDATE allocations SET status = 'released', released_at = now(), reusable_at = now(),
				reserved_until = NULL, updated_at = now()
			WHERE id IN (
				SELECT id FROM allocations WHERE status = 'reserved' AND reserved_until < now()
				ORDER BY reserved_until LIMIT 1000 FOR UPDATE SKIP LOCKED
			)
			RETURNING id, pool_id, value, object_type, object_ref
		)
		INSERT INTO allocation_events (allocation_id, pool_id, value, action, from_status, to_status, object_type, object_ref, actor)
		SELECT id, pool_id, value, 'expire', 'reserved', 'released', object_type, object_ref, 'system' FROM expired`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ---- Sorgular ---------------------------------------------------------------

type AllocationFilter struct {
	PoolID     int64
	Status     string
	ObjectType string
	ObjectRef  string
	Q          string
	Limit      int
	Offset     int
}

type AllocationPage struct {
	Items []Allocation `json:"items"`
	Total int64        `json:"total"`
}

func (s *Store) ListAllocations(ctx context.Context, f AllocationFilter) (*AllocationPage, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if f.PoolID != 0 {
		add("a.pool_id = ?", f.PoolID)
	}
	if f.Status != "" {
		add("a.status = ?", f.Status)
	}
	if f.ObjectType != "" {
		add("a.object_type = ?", f.ObjectType)
	}
	if f.ObjectRef != "" {
		add("a.object_ref = ?", f.ObjectRef)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		add("(a.value ILIKE ? || '%' OR a.object_ref ILIKE '%' || ? || '%')", esc)
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	sql := `SELECT ` + allocColumns + `, count(*) OVER () ` + allocFrom
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit, f.Offset)
	sql += fmt.Sprintf(" ORDER BY a.updated_at DESC, a.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &AllocationPage{Items: []Allocation{}}
	for rows.Next() {
		var a Allocation
		var meta []byte
		if err := rows.Scan(&a.ID, &a.PoolID, &a.PoolCode, &a.PoolName, &a.Kind, &a.KindName, &a.Value, &a.Status, &a.ObjectType, &a.ObjectRef,
			&a.Note, &meta, &a.ReservedUntil, &a.ReusableAt, &a.AssignedAt, &a.ReleasedAt, &a.CreatedAt, &a.UpdatedAt,
			&page.Total); err != nil {
			return nil, err
		}
		a.Metadata = json.RawMessage(meta)
		page.Items = append(page.Items, a)
	}
	return page, rows.Err()
}

type AllocationDetail struct {
	Allocation
	Events []Event `json:"events"`
}

func (s *Store) GetAllocation(ctx context.Context, id int64) (*AllocationDetail, error) {
	a, err := scanAllocation(s.db.QueryRow(ctx, `SELECT `+allocColumns+allocFrom+` WHERE a.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFoundf("tahsis bulunamadı")
	}
	if err != nil {
		return nil, err
	}
	events, err := s.ListEvents(ctx, EventFilter{AllocationID: id, Limit: 200})
	if err != nil {
		return nil, err
	}
	return &AllocationDetail{*a, events}, nil
}

type EventFilter struct {
	PoolID       int64
	AllocationID int64
	Limit        int
}

func (s *Store) ListEvents(ctx context.Context, f EventFilter) ([]Event, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT e.id, e.allocation_id, e.pool_id, p.code, e.value, e.action, e.from_status, e.to_status,
			e.object_type, e.object_ref, e.actor, e.note, e.created_at
		FROM allocation_events e JOIN pools p ON p.id = e.pool_id
		WHERE ($1 = 0 OR e.pool_id = $1) AND ($2 = 0 OR e.allocation_id = $2)
		ORDER BY e.id DESC LIMIT $3`, f.PoolID, f.AllocationID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.AllocationID, &e.PoolID, &e.PoolCode, &e.Value, &e.Action, &e.FromStatus, &e.ToStatus,
			&e.ObjectType, &e.ObjectRef, &e.Actor, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LookupMatch bir değerin hangi havuza düştüğünü ve şu anki durumunu anlatır.
type LookupMatch struct {
	PoolID     int64       `json:"pool_id"`
	PoolCode   string      `json:"pool_code"`
	PoolName   string      `json:"pool_name"`
	Value      string      `json:"value"`
	State      string      `json:"state"` // available | reusable | reserved | assigned | released | blocked
	Allocation *Allocation `json:"allocation"`
}

// Lookup değeri her havuzun kurallarıyla çözmeyi dener; aralığına düştüğü havuzları döndürür.
func (s *Store) Lookup(ctx context.Context, value string) ([]LookupMatch, error) {
	rows, err := s.db.Query(ctx, `SELECT `+poolColumns+`, r.start_value, r.end_value`+poolFrom+`
		JOIN pool_ranges r ON r.pool_id = p.id ORDER BY p.id, r.start_value`)
	if err != nil {
		return nil, err
	}
	type hit struct {
		pool Pool
		base int64
	}
	var hits []hit
	seen := map[int64]bool{}
	for rows.Next() {
		var r poolRow
		var start, end int64
		if err := rows.Scan(append(r.targets(), &start, &end)...); err != nil {
			rows.Close()
			return nil, err
		}
		p, err := r.finish()
		if err != nil {
			rows.Close()
			return nil, err
		}
		if seen[p.ID] {
			continue
		}
		base, err := p.Codec().Parse(value)
		if err == nil && base >= start && base <= end {
			seen[p.ID] = true
			hits = append(hits, hit{*p, base})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []LookupMatch{}
	for _, h := range hits {
		m := LookupMatch{PoolID: h.pool.ID, PoolCode: h.pool.Code, PoolName: h.pool.Name,
			Value: h.pool.Codec().Format(h.base), State: "available"}
		a, err := scanAllocation(s.db.QueryRow(ctx, `SELECT `+allocColumns+allocFrom+` WHERE a.pool_id = $1 AND a.base_value = $2`,
			h.pool.ID, h.base))
		switch {
		case err == nil:
			m.Allocation = a
			m.State = a.Status
			if a.Status == StatusReleased && a.ReusableAt != nil && !a.ReusableAt.After(time.Now()) {
				m.State = "reusable" // daha önce kullanılmış, tekrar dağıtılabilir
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
