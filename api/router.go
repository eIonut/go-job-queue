package api

import "net/http"

func NewRouter(handler *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /jobs", handler.CreateJob)
	mux.HandleFunc("GET /jobs", handler.GetJobs)
	mux.HandleFunc("GET /jobs/{id}", handler.GetJob)

	return mux
}
