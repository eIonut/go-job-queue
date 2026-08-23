package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"go-job-queue/api"
	"go-job-queue/database/db"
	"go-job-queue/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(
		ctx,
		"postgres://postgres:password@localhost:5433/jobqueue?sslmode=disable",
	)
	if err != nil {
		panic(err)
	}

	defer pool.Close()

	queries := db.New(pool)

	for range 5 {
		go worker.StartWorker(ctx, queries)
	}

	handler := api.NewHandler(queries)
	router := api.NewRouter(handler)

	port := os.Getenv("PORT")

	if port == "" {
		port = "5001"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	fmt.Println("Server listening on port:", port)

	err = server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
	}
}
