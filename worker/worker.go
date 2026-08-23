package worker

import (
	"context"
	"errors"
	"fmt"
	"go-job-queue/database/db"
	"time"

	"github.com/jackc/pgx/v5"
)

type Worker struct {
	queries      *db.Queries
	maxAttempts  int
	pollInterval time.Duration
}

func NewWorker(
	queries *db.Queries,
	maxAttempts int,
	pollInterval time.Duration,
) *Worker {
	return &Worker{
		queries:      queries,
		maxAttempts:  maxAttempts,
		pollInterval: pollInterval,
	}
}

func (w *Worker) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Worker stopped")
			return
		default:
		}

		processed, err := w.ProcessOneJob(ctx)

		if errors.Is(err, context.Canceled) {
			return
		}

		if err != nil {
			fmt.Println("Worker error:", err)

			select {
			case <-ctx.Done():
				return
			case <-time.After(w.pollInterval):
			}

			continue
		}

		if !processed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.pollInterval):
			}
		}
	}
}

func (w *Worker) ProcessJob(job db.Job) error {
	fmt.Printf(
		"Processing job %d, attempt %d/%d\n",
		job.ID,
		job.Attempts+1,
		w.maxAttempts,
	)

	time.Sleep(3 * time.Second)

	if job.Type == "fail_test" {
		return errors.New("simulated job failure")
	}

	return nil
}

func (w *Worker) ProcessOneJob(ctx context.Context) (bool, error) {
	job, err := w.queries.ClaimPendingJob(ctx)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	// From this point onward, the job has already been claimed.
	// Give it its own context so it can finish even if shutdown starts.
	jobCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	err = w.ProcessJob(job)
	if err != nil {
		markErr := w.queries.MarkJobFailed(jobCtx, job.ID)
		if markErr != nil {
			return false, markErr
		}

		if job.Attempts+1 < int32(w.maxAttempts) {
			err = w.queries.ScheduleRetry(jobCtx, job.ID)
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

	err = w.queries.MarkJobCompleted(jobCtx, job.ID)
	if err != nil {
		return false, err
	}

	fmt.Println("Job completed:", job.ID)

	return true, nil
}
