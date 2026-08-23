package worker

import (
	"context"
	"errors"
	"fmt"
	"go-job-queue/database/db"
	"time"

	"github.com/jackc/pgx/v5"
)

func StartWorker(ctx context.Context, queries *db.Queries) {
	for {
		processed, err := ProcessOneJob(ctx, queries)
		if err != nil {
			fmt.Println("Worker error:", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if !processed {
			time.Sleep(1 * time.Second)
		}
	}
}

func ProcessOneJob(ctx context.Context, queries *db.Queries) (bool, error) {
	job, err := queries.ClaimPendingJob(ctx)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	fmt.Println("Processing job:", job.ID)

	time.Sleep(3 * time.Second)

	err = queries.MarkJobCompleted(ctx, job.ID)
	if err != nil {
		return false, err
	}

	fmt.Println("Job completed:", job.ID)

	return true, nil
}
