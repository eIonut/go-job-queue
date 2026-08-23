package worker

import (
	"context"
	"errors"
	"fmt"
	"go-job-queue/database/db"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxAttempts = 3

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

func ProcessJob(job db.Job) error {
	fmt.Printf(
		"Processing job %d, attempt %d/%d\n",
		job.ID,
		job.Attempts+1,
		maxAttempts,
	)

	time.Sleep(3 * time.Second)

	if job.Type == "fail_test" {
		return errors.New("simulated job failure")
	}

	return nil
}

func ProcessOneJob(ctx context.Context, queries *db.Queries) (bool, error) {
	job, err := queries.ClaimPendingJob(ctx)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	err = ProcessJob(job)
	if err != nil {
		markErr := queries.MarkJobFailed(ctx, job.ID)
		if markErr != nil {
			return false, markErr
		}

		if job.Attempts+1 < maxAttempts {
			err = queries.ScheduleRetry(ctx, job.ID)
			if err != nil {
				return false, err
			}
		}

		fmt.Printf(
			"Job %d failed on attempt %d\n",
			job.ID,
			job.Attempts+1,
		)

		return true, nil
	}

	err = queries.MarkJobCompleted(ctx, job.ID)
	if err != nil {
		return false, err
	}

	fmt.Println("Job completed:", job.ID)

	return true, nil
}
