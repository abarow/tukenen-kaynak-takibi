package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kaynak/internal/kinds"
)

// Tahsis durumları.
const (
	StatusReserved = "reserved" // geçici tutuluyor, süresi dolunca boşa düşer
	StatusAssigned = "assigned" // nesneye kalıcı olarak atanmış
	StatusReleased = "released" // geri bırakılmış; reusable_at'e göre tekrar dağıtılabilir
	StatusBlocked  = "blocked"  // elle dağıtım dışı bırakılmış
)

// Yeniden kullanım politikaları.
const (
	ReuseNever     = "never"
	ReuseCooldown  = "cooldown"
	ReuseImmediate = "immediate"
)

type Pool struct {
	ID                    int64      `json:"id"`
	Code                  string     `json:"code"`
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	Kind                  kinds.Kind `json:"kind"`
	Prefix                string     `json:"prefix"`
	PadWidth              int        `json:"pad_width"`
	ReusePolicy           string     `json:"reuse_policy"`
	CooldownDays          int        `json:"cooldown_days"`
	ReservationTTLSeconds int        `json:"reservation_ttl_seconds"`
	LowWatermark          int64      `json:"low_watermark"`
	Active                bool       `json:"active"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CustomKind            *string    `json:"custom_kind"` // özel tipin kodu (kind = custom ise)
	KindName              string     `json:"kind_name"`
	Stats                 PoolStats  `json:"stats"`
	Ranges                []Range    `json:"ranges,omitempty"`

	scheme *kinds.Scheme
}

func (p *Pool) Codec() kinds.Codec {
	if p.scheme != nil {
		return kinds.NewCustomCodec(p.scheme)
	}
	c, err := kinds.NewCodec(p.Kind, p.Prefix, p.PadWidth)
	if err != nil {
		// Veritabanındaki havuzlar oluşturulurken doğrulandı; buraya düşmek bir hata.
		panic(fmt.Sprintf("havuz %d için codec kurulamadı: %v", p.ID, err))
	}
	return c
}

type PoolStats struct {
	Capacity      int64 `json:"capacity"`
	CapacityExact bool  `json:"capacity_exact"` // false: kurallı tipte büyük aralık, örneklem tahmini
	Assigned      int64 `json:"assigned"`
	Reserved      int64 `json:"reserved"`
	Released      int64 `json:"released"` // bırakılmış ama henüz tekrar kullanılamaz
	Reusable      int64 `json:"reusable"` // bırakılmış ve tekrar dağıtılabilir
	Blocked       int64 `json:"blocked"`
	Available     int64 `json:"available"`
	Low           bool  `json:"low"`
}

type Range struct {
	ID         int64     `json:"id"`
	PoolID     int64     `json:"pool_id"`
	Start      int64     `json:"start"`
	End        int64     `json:"end"`
	Next       int64     `json:"next"`
	Size       int64     `json:"size"`
	Valid      int64     `json:"valid"`       // kurallara uyan değer sayısı (yerleşik tiplerde = size)
	ValidExact bool      `json:"valid_exact"` // false: örneklem tahmini
	Untouched  int64     `json:"untouched"`   // imlecin henüz ulaşmadığı gövde sayısı
	FirstValue string    `json:"first_value"`
	LastValue  string    `json:"last_value"`
	CreatedAt  time.Time `json:"created_at"`
}

type Allocation struct {
	ID            int64           `json:"id"`
	PoolID        int64           `json:"pool_id"`
	PoolCode      string          `json:"pool_code"`
	PoolName      string          `json:"pool_name"`
	Kind          kinds.Kind      `json:"kind"`
	KindName      string          `json:"kind_name"` // özel tiplerde tipin adı
	Value         string          `json:"value"`
	Status        string          `json:"status"`
	ObjectType    *string         `json:"object_type"`
	ObjectRef     *string         `json:"object_ref"`
	Note          string          `json:"note"`
	Metadata      json.RawMessage `json:"metadata"`
	ReservedUntil *time.Time      `json:"reserved_until"`
	ReusableAt    *time.Time      `json:"reusable_at"`
	AssignedAt    *time.Time      `json:"assigned_at"`
	ReleasedAt    *time.Time      `json:"released_at"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Event struct {
	ID           int64     `json:"id"`
	AllocationID int64     `json:"allocation_id"`
	PoolID       int64     `json:"pool_id"`
	PoolCode     string    `json:"pool_code"`
	Value        string    `json:"value"`
	Action       string    `json:"action"`
	FromStatus   *string   `json:"from_status"`
	ToStatus     string    `json:"to_status"`
	ObjectType   *string   `json:"object_type"`
	ObjectRef    *string   `json:"object_ref"`
	Actor        string    `json:"actor"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

// ---- Hatalar ----------------------------------------------------------------

type ErrorKind int

const (
	KindInvalid ErrorKind = iota + 1
	KindNotFound
	KindConflict
)

// Error, kullanıcıya gösterilebilir iş kuralı hatası.
type Error struct {
	Kind    ErrorKind
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func invalidf(format string, args ...any) error {
	return &Error{KindInvalid, "invalid", fmt.Sprintf(format, args...)}
}

func notFoundf(format string, args ...any) error {
	return &Error{KindNotFound, "not_found", fmt.Sprintf(format, args...)}
}

func conflictf(code, format string, args ...any) error {
	return &Error{KindConflict, code, fmt.Sprintf(format, args...)}
}

// fromKindsErr kinds paketinin doğrulama hatasını iş hatasına çevirir.
func fromKindsErr(err error) error {
	if errors.Is(err, kinds.ErrInvalid) {
		return &Error{KindInvalid, "invalid_value", err.Error()}
	}
	return err
}
