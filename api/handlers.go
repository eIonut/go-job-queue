package api

import (
	"encoding/json"
	"errors"
	"go-job-queue/database/db"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type Handler struct {
	queries *db.Queries
}

func NewHandler(queries *db.Queries) *Handler {
	return &Handler{
		queries: queries,
	}
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	job, err := h.queries.CreateJob(r.Context(), db.CreateJobParams{
		Type:    req.Type,
		Payload: req.Payload,
	})
	if err != nil {
		http.Error(w, "failed to create job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	err = json.NewEncoder(w).Encode(job)

	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	idString := r.PathValue("id")

	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}

	job, err := h.queries.GetJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "failed to get job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(job)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) GetJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.queries.GetJobs(r.Context())
	if err != nil {
		http.Error(w, "failed to get jobs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(jobs)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
