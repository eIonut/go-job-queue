package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go-job-queue/api"
	"go-job-queue/config"
	"go-job-queue/database/db"
	"go-job-queue/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

func startServer(server *http.Server, port string) {
	fmt.Println("Server listening on port:", port)

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		fmt.Println("Server error:", err)
	}
}

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)

	if err != nil {
		panic(err)
	}
	defer pool.Close()

	queries := db.New(pool)

	var wg sync.WaitGroup

	for range cfg.WorkerCount {
		w := worker.NewWorker(
			queries,
			cfg.MaxAttempts,
			cfg.PollInterval,
		)

		wg.Add(1)

		go func() {
			defer wg.Done()
			w.Start(ctx)
		}()
	}

	handler := api.NewHandler(queries)
	router := api.NewRouter(handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5001"
	}

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go startServer(server, port)

	// Block main until Ctrl+C / SIGTERM.
	<-ctx.Done()

	fmt.Println("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = server.Shutdown(shutdownCtx)
	if err != nil {
		fmt.Println("Server shutdown error:", err)
	}

	fmt.Println("Waiting for workers to finish...")

	wg.Wait()

	fmt.Println("All workers stopped")
}
