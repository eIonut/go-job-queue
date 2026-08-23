# Go Job Queue

A lightweight background job processing service built from scratch in Go and PostgreSQL.

The project implements a persistent job queue with concurrent workers, atomic job claiming, automatic retries, delayed retry scheduling, and graceful shutdown.

I built this project primarily to deepen my understanding of Go concurrency and backend systems beyond traditional request/response web applications.

## Features

- REST API for creating background jobs
- PostgreSQL-backed persistent queue
- Concurrent worker pool using goroutines
- Configurable number of workers
- Atomic job claiming with PostgreSQL row locking
- `FOR UPDATE SKIP LOCKED` to prevent multiple workers from processing the same job
- Job lifecycle:
  - `pending`
  - `running`
  - `completed`
  - `failed`

- Automatic retry handling
- Configurable maximum retry attempts
- Delayed retries using `retry_at`
- PostgreSQL connection pooling with `pgxpool`
- SQL code generation using `sqlc`
- Database migrations
- Graceful shutdown using Go contexts and OS signals
- Configurable runtime settings through environment variables

## Architecture

```text
                    ┌─────────────────┐
                    │   Client/App    │
                    └────────┬────────┘
                             │
                        POST /jobs
                             │
                             ▼
                    ┌─────────────────┐
                    │     Go API      │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │   PostgreSQL    │
                    │                 │
                    │ pending jobs    │
                    └────────┬────────┘
                             │
              ClaimPendingJob
              FOR UPDATE
              SKIP LOCKED
                             │
             ┌───────────────┼───────────────┐
             ▼               ▼               ▼
        ┌─────────┐     ┌─────────┐     ┌─────────┐
        │Worker 1 │     │Worker 2 │ ... │Worker N │
        └────┬────┘     └────┬────┘     └────┬────┘
             │               │               │
             ▼               ▼               ▼
          process          process          process
             │
             ├── success ───────────────► completed
             │
             └── failure
                    │
                    ▼
                 failed
                    │
             attempts < max?
                    │
                  yes
                    ▼
              retry scheduled
                    │
                retry_at
                    │
                    ▼
                 pending
```

## Job Lifecycle

A newly created job starts as:

```text
pending
```

A worker atomically claims it:

```text
pending → running
```

If processing succeeds:

```text
running → completed
```

If processing fails:

```text
running → failed
```

If the maximum number of attempts has not been reached, the job is scheduled for retry:

```text
failed → pending
```

with a future `retry_at` timestamp.

Once the maximum number of attempts is reached, the job remains:

```text
failed
```

and is no longer automatically processed.

## Preventing Duplicate Processing

Multiple workers may query PostgreSQL at the same time.

A naive implementation could allow two workers to select the same `pending` job.

This project prevents that by atomically claiming jobs using PostgreSQL row locking:

```sql
WITH next_job AS (
    SELECT id
    FROM jobs
    WHERE status = 'pending'
      AND (retry_at IS NULL OR retry_at <= NOW())
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs
SET status = 'running',
    updated_at = NOW()
WHERE id = (SELECT id FROM next_job)
RETURNING *;
```

`FOR UPDATE` locks the selected row while `SKIP LOCKED` allows other workers to skip already claimed jobs and process another available job instead.

This allows several workers to safely process the queue concurrently.

## Worker Pool

The application runs multiple workers concurrently using goroutines.

```go
for range cfg.WorkerCount {
    worker := worker.NewWorker(
        queries,
        cfg.MaxAttempts,
        cfg.PollInterval,
    )

    go worker.Start(ctx)
}
```

Each worker continuously attempts to claim jobs.

When the queue is empty, workers wait for the configured polling interval before checking again.

If jobs are available, workers continue processing without an unnecessary delay between jobs.

## PostgreSQL Connection Pool

The application uses `pgxpool` instead of a single PostgreSQL connection.

Multiple workers and HTTP handlers can access the database concurrently:

```text
Worker 1 ─┐
Worker 2 ─┤
Worker 3 ─┼──► pgxpool ───► PostgreSQL
Worker 4 ─┤
HTTP API ─┘
```

Connections are reused rather than opened for every query.

## Retries

Failed jobs increment their `attempts` count.

Instead of blocking a worker with `time.Sleep()` while waiting to retry, retries are scheduled in PostgreSQL:

```text
retry_at = NOW() + retry delay
```

Workers only claim jobs where:

```sql
retry_at IS NULL OR retry_at <= NOW()
```

This keeps workers available to process other jobs while failed jobs wait for their retry time.

## Graceful Shutdown

The service listens for operating system shutdown signals such as `SIGINT` and `SIGTERM`.

For example:

```text
Ctrl+C
  ↓
cancel application context
  ↓
stop accepting new work
  ↓
workers stop claiming new jobs
  ↓
already-running jobs finish
  ↓
HTTP server shuts down
  ↓
database pool closes
```

A `sync.WaitGroup` is used to wait for worker goroutines before the process exits.

This prevents jobs from being abandoned unnecessarily during application shutdown.

## Tech Stack

- Go
- PostgreSQL
- pgx / pgxpool
- sqlc
- Docker Compose
- golang-migrate
- Go standard library HTTP server

No web framework is used for the API.

## Project Structure

```text
.
├── api/
│   ├── handler.go
│   └── router.go
│
├── config/
│   └── config.go
│
├── database/
│   ├── db/
│   │   └── generated sqlc code
│   ├── migrations/
│   └── queries/
│       └── jobs.sql
│
├── worker/
│   └── worker.go
│
├── docker-compose.yml
├── go.mod
├── sqlc.yaml
└── main.go
```

## Configuration

The service is configured using environment variables.

| Variable                | Default              | Description                      |
| ----------------------- | -------------------- | -------------------------------- |
| `PORT`                  | `5001`               | HTTP server port                 |
| `DATABASE_URL`          | local PostgreSQL URL | PostgreSQL connection string     |
| `WORKER_COUNT`          | `5`                  | Number of concurrent workers     |
| `MAX_ATTEMPTS`          | `3`                  | Maximum attempts per job         |
| `POLL_INTERVAL_SECONDS` | `1`                  | Delay when no jobs are available |

Example:

```bash
PORT=5001 \
WORKER_COUNT=5 \
MAX_ATTEMPTS=3 \
POLL_INTERVAL_SECONDS=1 \
DATABASE_URL="postgres://postgres:password@localhost:5433/jobqueue?sslmode=disable" \
go run .
```

## Running Locally

### 1. Start PostgreSQL

```bash
docker compose up -d
```

### 2. Run migrations

```bash
migrate \
  -path database/migrations \
  -database "postgres://postgres:password@localhost:5433/jobqueue?sslmode=disable" \
  up
```

### 3. Generate database code

```bash
sqlc generate
```

### 4. Start the service

```bash
go run .
```

The API will be available at:

```text
http://localhost:5001
```

## Creating a Job

```bash
curl http://localhost:5001/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "send_email",
    "payload": {
      "to": "test@example.com"
    }
  }'
```

Example lifecycle:

```text
Job created
    ↓
pending
    ↓
claimed by worker
    ↓
running
    ↓
processed
    ↓
completed
```

## Simulating Failure

A `fail_test` job can be used during development to test retry behavior:

```bash
curl http://localhost:5001/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "fail_test",
    "payload": {}
  }'
```

The worker intentionally fails this job, allowing the retry and `maxAttempts` behavior to be tested.

Example:

```text
Processing job 28, attempt 1/3
Job 28 failed on attempt 1

Processing job 28, attempt 2/3
Job 28 failed on attempt 2

Processing job 28, attempt 3/3
Job 28 failed on attempt 3
```

The job then remains permanently in the `failed` state unless manually retried.

## What I Learned

This project was built as a hands-on exercise in backend and systems programming with Go.

Key concepts explored:

### Go

- Goroutines
- Concurrent worker pools
- `context.Context`
- Cancellation propagation
- Graceful shutdown
- `select`
- Error handling
- Structs and methods
- Dependency injection through constructors
- `sync.WaitGroup`
- Packages and project organization

### Backend Engineering

- Background job processing
- Asynchronous workloads
- Persistent queues
- Job lifecycle design
- Retry strategies
- Delayed retries
- Failure handling
- Database migrations
- Configuration through environment variables
- HTTP API design
- Connection pooling

### PostgreSQL

- Row-level locking
- `FOR UPDATE`
- `SKIP LOCKED`
- Common Table Expressions (`WITH`)
- Atomic job claiming
- Concurrent database access
- PostgreSQL-backed queues

### Reliability

The project also introduced several problems that become important in real backend systems:

- preventing duplicate job processing
- safely handling multiple concurrent workers
- avoiding blocked workers during retry delays
- limiting retries
- persisting work across application restarts
- shutting down without abandoning active jobs

## Possible Future Improvements

The current version intentionally focuses on the core job queue mechanics.

Possible future additions include:

- Job priorities
- Scheduled jobs
- Exponential retry backoff
- Dead-letter queue
- Manual retry endpoint
- Job cancellation
- Metrics and monitoring
- Structured logging
- PostgreSQL `LISTEN/NOTIFY` instead of polling
- Authentication
- Prometheus metrics
- Distributed workers running on multiple service instances

## Why This Project

Most web applications hide concurrency and background processing behind frameworks or external queue services.

For this project, I intentionally implemented the core mechanics directly using Go and PostgreSQL to better understand:

- how workers coordinate
- how background jobs are persisted
- how race conditions occur
- how database locking prevents duplicate processing
- how retries should be scheduled
- how concurrent services manage database connections
- how long-running processes shut down safely

The goal was not to replace mature systems such as RabbitMQ, Kafka, or production job queue platforms, but to understand the engineering concepts behind them.
