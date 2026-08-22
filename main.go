package main

import (
	"context"
	"fmt"
	"net/http"

	"go-job-queue/api"
	"go-job-queue/database/db"

	"github.com/jackc/pgx/v5"
)

func main() {
	conn, err := pgx.Connect(
		context.Background(),
		"postgres://postgres:password@localhost:5432/jobqueue",
	)
	if err != nil {
		panic(err)
	}

	defer conn.Close(context.Background())

	queries := db.New(conn)

	handler := api.NewHandler(queries)
	router := api.NewRouter(handler)

	server := &http.Server{
		Addr:    ":5000",
		Handler: router,
	}

	err = server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
	}
}