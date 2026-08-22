package main

import (
	"fmt"

	"go-job-queue/database"
)

func main() {
	fmt.Println("Job queue started")
	database.Something()
}