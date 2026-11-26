// Package api REST uç noktalarını ve gömülü arayüzü sunar.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"kaynak/internal/kinds"
	"kaynak/internal/store"
)

type Server struct {
	store *store.Store
}

// New tüm rotaları kurar. ui nil ise yalnızca API sunulur.
func New(st *store.Store, ui fs.FS) http.Handler {
	s := &Server{store: st}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/kinds", s.listKinds)
	mux.HandleFunc("GET /api/custom-kinds", s.listCustomKinds)
	mux.HandleFunc("POST /api/custom-kinds", s.createCustomKind)
	mux.HandleFunc("POST /api/custom-kinds/test", s.testCustomKind)
	mux.HandleFunc("DELETE /api/custom-kinds/{id}", s.deleteCustomKind)
	mux.HandleFunc("POST /api/tools/validate", s.validate)
	mux.HandleFunc("GET /api/lookup", s.lookup)

	mux.HandleFunc("GET /api/pools", s.listPools)
	mux.HandleFunc("POST /api/pools", s.createPool)
	mux.HandleFunc("GET /api/pools/{pool}", s.getPool)
	mux.HandleFunc("PATCH /api/pools/{pool}", s.updatePool)
	mux.HandleFunc("DELETE /api/pools/{pool}", s.deletePool)
	mux.HandleFunc("POST /api/pools/{pool}/ranges", s.addRange)
	mux.HandleFunc("DELETE /api/pools/{pool}/ranges/{range}", s.deleteRange)
	mux.HandleFunc("POST /api/pools/{pool}/allocate", s.allocate)
	mux.HandleFunc("POST /api/pools/{pool}/allocate/bulk", s.allocateBulk)
	mux.HandleFunc("POST /api/pools/{pool}/block", s.block)

	mux.HandleFunc("GET /api/allocations", s.listAllocations)
	mux.HandleFunc("GET /api/allocations/{id}", s.getAllocation)
	mux.HandleFunc("POST /api/allocations/{id}/confirm", s.confirm)
	mux.HandleFunc("POST /api/allocations/{id}/release", s.release)

	mux.HandleFunc("GET /api/events", s.listEvents)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, &store.Error{Kind: store.KindNotFound, Code: "not_found", Message: "uç nokta bulunamadı"})
	})
	if ui != nil {
		mux.Handle("/", spaHandler(ui))
	}
	return withMiddleware(mux)
}

// ---- Yardımcılar ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeErr(w http.ResponseWriter, err error) {
	var body errorBody
	status := http.StatusInternalServerError
	var e *store.Error
	switch {
	case errors.As(err, &e):
		body.Error.Code, body.Error.Message = e.Code, e.Message
		switch e.Kind {
		case store.KindInvalid:
			status = http.StatusBadRequest
		case store.KindNotFound:
			status = http.StatusNotFound
		case store.KindConflict:
			status = http.StatusConflict
		}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = http.StatusServiceUnavailable
		body.Error.Code, body.Error.Message = "timeout", "istek zaman aşımına uğradı"
	default:
		slog.Error("beklenmeyen hata", "err", err)
		body.Error.Code, body.Error.Message = "internal", "sunucu hatası"
	}
	writeJSON(w, status, body)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return &store.Error{Kind: store.KindInvalid, Code: "invalid_json", Message: "istek gövdesi boş"}
		}
		return &store.Error{Kind: store.KindInvalid, Code: "invalid_json", Message: "geçersiz JSON: " + err.Error()}
	}
	return nil
}

// decodeOptional boş gövdeye izin verir (ör. not alanı opsiyonel olan işlemler).
func decodeOptional(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	err := decode(r, v)
	var e *store.Error
	if errors.As(err, &e) && e.Message == "istek gövdesi boş" {
		return nil
	}
	return err
}

func actor(r *http.Request) string {
	a := strings.TrimSpace(r.Header.Get("X-Actor"))
	if a == "" {
		return "api"
	}
	if len(a) > 64 {
		a = a[:64]
	}
	return a
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, &store.Error{Kind: store.KindInvalid, Code: "invalid_id", Message: name + " geçersiz"}
	}
	return id, nil
}

// poolID havuzu sayısal kimlik veya kod ile kabul eder: /api/pools/12 ya da /api/pools/gs1-ana
func (s *Server) poolID(r *http.Request) (int64, error) {
	return s.store.ResolvePool(r.Context(), r.PathValue("pool"))
}

func queryInt(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return v
}

// ---- Genel ------------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listKinds(w http.ResponseWriter, r *http.Request) {
	infos, err := s.store.KindInfos(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind       kinds.Kind `json:"kind"`
		CustomKind string     `json:"custom_kind"`
		Value      string     `json:"value"`
		Prefix     string     `json:"prefix"`
		PadWidth   int        `json:"pad_width"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	var c kinds.Codec
	var err error
	if in.Kind == kinds.Custom {
		c, err = s.store.CustomCodec(r.Context(), in.CustomKind)
	} else {
		c, err = kinds.NewCodec(in.Kind, in.Prefix, in.PadWidth)
		if err != nil {
			err = &store.Error{Kind: store.KindInvalid, Code: "invalid_kind", Message: err.Error()}
		}
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.Validate(in.Value))
}

// ---- Özel kod tipleri ---------------------------------------------------------

func (s *Server) listCustomKinds(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCustomKinds(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "functions": kinds.FunctionHelp})
}

func (s *Server) createCustomKind(w http.ResponseWriter, r *http.Request) {
	var in store.CustomKindInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	k, err := s.store.CreateCustomKind(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// testCustomKind tanımı kaydetmeden dener: derleme hataları, örnek kodlar, yoğunluk
// ve istenirse bir değerin bu tanıma göre doğrulanması.
func (s *Server) testCustomKind(w http.ResponseWriter, r *http.Request) {
	var in struct {
		store.CustomKindInput
		Value string `json:"value"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, store.TestCustomKind(in.CustomKindInput, in.Value))
}

func (s *Server) deleteCustomKind(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.DeleteCustomKind(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	v := strings.TrimSpace(r.URL.Query().Get("value"))
	if v == "" {
		writeErr(w, &store.Error{Kind: store.KindInvalid, Code: "invalid", Message: "value parametresi zorunlu"})
		return
	}
	matches, err := s.store.Lookup(r.Context(), v)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": v, "matches": matches})
}

// ---- Havuzlar ---------------------------------------------------------------

func (s *Server) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.store.ListPools(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

func (s *Server) createPool(w http.ResponseWriter, r *http.Request) {
	var in store.CreatePoolInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.store.CreatePool(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getPool(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.store.GetPool(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updatePool(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in store.UpdatePoolInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.store.UpdatePool(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deletePool(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.DeletePool(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addRange(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in store.RangeInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.store.AddRange(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) deleteRange(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	rid, err := pathID(r, "range")
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.store.DeleteRange(r.Context(), id, rid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---- Tahsis -----------------------------------------------------------------

func (s *Server) allocate(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in store.AllocateRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, created, err := s.store.Allocate(r.Context(), id, in, actor(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK // idempotent tekrar: mevcut tahsis döndü
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"allocation": a, "created": created})
}

func (s *Server) allocateBulk(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Items []store.AllocateRequest `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.store.AllocateBulk(r.Context(), id, in.Items, actor(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": res})
}

func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	id, err := s.poolID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Value string `json:"value"`
		Note  string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.store.Block(r.Context(), id, in.Value, in.Note, actor(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) listAllocations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AllocationFilter{
		PoolID:     queryInt(r, "pool_id"),
		Status:     q.Get("status"),
		ObjectType: q.Get("object_type"),
		ObjectRef:  q.Get("object_ref"),
		Q:          q.Get("q"),
		Limit:      int(queryInt(r, "limit")),
		Offset:     int(queryInt(r, "offset")),
	}
	page, err := s.store.ListAllocations(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getAllocation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.store.GetAllocation(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type noteInput struct {
	Note string `json:"note"`
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	s.transition(w, r, s.store.Confirm)
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	s.transition(w, r, s.store.Release)
}

func (s *Server) transition(w http.ResponseWriter, r *http.Request,
	fn func(ctx context.Context, id int64, note, actor string) (*store.Allocation, error)) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, err)
		return
	}
	var in noteInput
	if err := decodeOptional(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, err := fn(r.Context(), id, in.Note, actor(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListEvents(r.Context(), store.EventFilter{
		PoolID: queryInt(r, "pool_id"),
		Limit:  int(queryInt(r, "limit")),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ---- Arayüz (SPA) -----------------------------------------------------------

func spaHandler(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if st, err := fs.Stat(ui, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// Bilinmeyen yollar SvelteKit'in istemci tarafı yönlendiricisine bırakılır.
		page, err := fs.ReadFile(ui, "200.html")
		if err != nil {
			http.Error(w, "Arayüz derlenmemiş. frontend klasöründe `npm run build` çalıştırıp sunucuyu yeniden derleyin.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(page)
	})
}

// ---- Middleware -------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", v, "path", r.URL.Path)
				writeErr(rec, errors.New("panic"))
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				slog.Info("istek", "method", r.Method, "path", r.URL.Path, "status", rec.status,
					"ms", time.Since(start).Milliseconds())
			}
		}()
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next.ServeHTTP(rec, r)
	})
}
