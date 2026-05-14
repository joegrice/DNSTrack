package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/joe/dnstrack/internal/config"
	"github.com/joe/dnstrack/internal/scheduler"
	"github.com/joe/dnstrack/internal/store"
)

type Handler struct {
	store     *store.Store
	scheduler *scheduler.Scheduler
	cfg       *config.Config
}

func New(st *store.Store, sch *scheduler.Scheduler, cfg *config.Config) *Handler {
	return &Handler{store: st, scheduler: sch, cfg: cfg}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/runs/latest", h.getLatestRun)
	r.Get("/runs/{id}", h.getRun)
	r.Get("/runs", h.getRuns)
	r.Get("/history", h.getHistory)
	r.Get("/providers", h.getProviders)
	r.Post("/test", h.triggerTest)
	r.Get("/settings", h.getSettings)
	r.Put("/settings", h.updateSettings)

	return r
}

func (h *Handler) getLatestRun(w http.ResponseWriter, r *http.Request) {
	detail, err := h.store.GetLatestRunDetail()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "no runs yet"})
		return
	}

	// Inject provider config (IPs, DoH URL) into response
	enriched := h.enrichRunDetail(detail)
	writeJSON(w, http.StatusOK, enriched)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	detail, err := h.store.GetRunDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	enriched := h.enrichRunDetail(detail)
	writeJSON(w, http.StatusOK, enriched)
}

func (h *Handler) getRuns(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	total, err := h.store.GetRunCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	runs, err := h.store.GetRuns(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type runSummary struct {
		store.Run
		ProviderCount int `json:"provider_count"`
	}

	var summaries []runSummary
	for _, run := range runs {
		detail, err := h.store.GetRunDetail(run.ID)
		if err != nil {
			log.Printf("[api] error getting run detail for run %d: %v", run.ID, err)
			continue
		}
		summaries = append(summaries, runSummary{
			Run:           run,
			ProviderCount: len(detail.Providers),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs":  summaries,
		"total": total,
	})
}

func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider parameter is required")
		return
	}

	hours := 24
	if hrs := r.URL.Query().Get("hours"); hrs != "" {
		if v, err := strconv.Atoi(hrs); err == nil && v > 0 && v <= 2160 {
			hours = v
		}
	}

	bucketMinutes := 60
	if bm := r.URL.Query().Get("bucket"); bm != "" {
		if v, err := strconv.Atoi(bm); err == nil && v > 0 && v <= 10080 {
			bucketMinutes = v
		}
	}

	if provider == "all" {
		allPoints, err := h.store.GetHistoryAll(hours, bucketMinutes)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, allPoints)
		return
	}

	points, err := h.store.GetHistory(provider, hours, bucketMinutes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, points)
}

func (h *Handler) getProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.cfg.Providers)
}

func (h *Handler) triggerTest(w http.ResponseWriter, r *http.Request) {
	log.Println("[api] manual test triggered")
	go func() {
		if err := h.scheduler.RunTests(); err != nil {
			log.Printf("[api] manual test error: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "test started"})
}

func (h *Handler) enrichRunDetail(detail *store.RunDetail) map[string]interface{} {
	providerMap := make(map[string]config.Provider)
	for _, p := range h.cfg.Providers {
		providerMap[p.Name] = p
	}

	type enrichedProvider struct {
		store.ProviderResult
		IPs    []string `json:"ips"`
		DoHURL string   `json:"doh_url,omitempty"`
		Type   string   `json:"type"`
	}

	var enrichedProviders []enrichedProvider
	for _, pr := range detail.Providers {
		ep := enrichedProvider{ProviderResult: pr}
		if cfg, ok := providerMap[pr.Provider]; ok {
			ep.IPs = cfg.IPs
			ep.DoHURL = cfg.DoHURL
			ep.Type = cfg.Type
		}
		enrichedProviders = append(enrichedProviders, ep)
	}

	return map[string]interface{}{
		"run_id":     detail.Run.ID,
		"created_at": detail.Run.CreatedAt,
		"providers":  enrichedProviders,
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"cron": h.scheduler.GetInterval(),
	})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cron string `json:"cron"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Cron == "" {
		writeError(w, http.StatusBadRequest, "cron expression required")
		return
	}

	if err := h.scheduler.SetInterval(body.Cron); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("[api] cron interval updated to: %s", body.Cron)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"cron":   body.Cron,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
