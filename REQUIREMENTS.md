# Go Job Queue — MVP Requirements

## Goal

Build a standalone persistent job queue service in Go using PostgreSQL.

The system should allow clients to enqueue background jobs, process them concurrently using workers, retry failed jobs, and persist job state so jobs survive application restarts.

## Tech Stack

- Go
- PostgreSQL
- `pgx` / `pgxpool`
- `sqlc`
- Docker / Docker Compose
- Standard Go HTTP server

## Core Concepts to Practice

This project should help practice:

- Goroutines
- Worker pools
- Channels
- `context.Context`
- PostgreSQL transactions
- Row locking
- `FOR UPDATE SKIP LOCKED`
- Graceful shutdown
- Database connection pooling
- Retry logic
- Background processing
- HTTP APIs
- Persistent state
- Error handling

## Requirements

### 1. Job Model

A job should contain at least:

- `id`
- `type`
- `payload`
- `status`
- `attempts`
- `max_attempts`
- `created_at`
- `updated_at`

Optional fields that can be added if useful:

- `started_at`
- `completed_at`
- `failed_at`
- `last_error`

### 2. Job Statuses

Support the following statuses:

- `pending`
- `processing`
- `completed`
- `failed`

Typical lifecycle:

```text
pending
   |
   v
processing
   |
   +----> completed
   |
   +----> pending   (retry)
   |
   +----> failed    (max attempts reached)
```

### 3. PostgreSQL Persistence

- Store all jobs in PostgreSQL.
- Jobs must survive application restarts.
- Use migrations to create the database schema.
- Use `sqlc` for database queries.
- Use `pgx` or `pgxpool` for PostgreSQL connections.

### 4. Enqueue Jobs

Expose an HTTP endpoint:

```http
POST /jobs
```

Example request:

```json
{
  "type": "send_email",
  "payload": {
    "to": "user@example.com",
    "subject": "Hello"
  }
}
```

The API should:

- Validate the request.
- Insert the job into PostgreSQL.
- Set its initial status to `pending`.
- Set `attempts` to `0`.
- Apply a default `max_attempts` if none is provided.
- Return the created job ID.

Example response:

```json
{
  "id": 123,
  "status": "pending"
}
```

### 5. Get Job Status

Expose:

```http
GET /jobs/:id
```

Return information such as:

```json
{
  "id": 123,
  "type": "send_email",
  "status": "completed",
  "attempts": 1,
  "max_attempts": 3
}
```

Return `404` if the job does not exist.

### 6. Worker Pool

- Start a configurable number of worker goroutines.
- Each worker should continuously look for available jobs.
- Workers should process jobs concurrently.
- The worker count should come from configuration.

Example:

```text
PostgreSQL jobs table
        |
        v
+-------------------+
| Worker 1          |
| Worker 2          |
| Worker 3          |
| ...               |
+-------------------+
```

### 7. Safe Job Claiming

Multiple workers must never process the same job simultaneously.

Use a PostgreSQL transaction and row locking.

The job selection query should use:

```sql
FOR UPDATE SKIP LOCKED
```

Conceptually:

```text
Worker 1 locks Job A
Worker 2 skips Job A
Worker 2 locks Job B
Worker 3 locks Job C
```

When a worker claims a job:

- Select one `pending` job.
- Lock it.
- Change its status to `processing`.
- Increment its attempt count.
- Commit the transaction.
- Process the job.

### 8. Job Processing

For the MVP, implement a small set of fake/demo job handlers.

Example types:

```text
send_email
generate_report
resize_image
```

The handlers do not need to perform real integrations.

For example:

```go
func handleSendEmail(job Job) error {
    time.Sleep(2 * time.Second)
    return nil
}
```

The objective is to build the queue infrastructure, not the actual business logic.

### 9. Job Completion

If a job succeeds:

- Set status to `completed`.
- Store completion time if implemented.
- Clear any previous error information.

### 10. Failed Jobs and Retries

If processing returns an error:

```text
attempts < max_attempts
```

then:

- Set the job back to `pending`.
- Store the error.
- Allow another worker to retry it.

If:

```text
attempts >= max_attempts
```

then:

- Set the job to `failed`.
- Do not retry it again automatically.
- Store the final error.

Example:

```text
attempt 1 -> failed -> pending
attempt 2 -> failed -> pending
attempt 3 -> failed -> failed permanently
```

### 11. Worker Polling

If there are no jobs available:

- Workers should not spin continuously and consume 100% CPU.
- Wait for a short configurable interval before checking PostgreSQL again.

Example:

```go
time.Sleep(500 * time.Millisecond)
```

More advanced mechanisms such as PostgreSQL `LISTEN/NOTIFY` are out of scope for the MVP.

### 12. Context

Use `context.Context` for:

- Database queries.
- Worker lifecycle.
- Application shutdown.
- HTTP requests where applicable.

Workers should stop when the application context is cancelled.

Conceptually:

```text
main context
    |
    +--> HTTP server
    |
    +--> Worker 1
    +--> Worker 2
    +--> Worker 3
```

Cancelling the main context should eventually stop all components.

### 13. Graceful Shutdown

Handle signals such as:

```text
SIGINT
SIGTERM
```

For example when pressing:

```text
Ctrl+C
```

The application should:

- Stop accepting new HTTP requests.
- Signal workers to stop.
- Allow currently running work to finish where reasonable.
- Close PostgreSQL connections.
- Exit cleanly.

### 14. Configuration

Read configuration from environment variables.

Minimum configuration:

```text
PORT
DATABASE_URL
WORKERS
MAX_ATTEMPTS
POLL_INTERVAL
```

Example:

```env
PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5432/jobqueue
WORKERS=5
MAX_ATTEMPTS=3
POLL_INTERVAL=500ms
```

### 15. Error Handling

Handle cases such as:

- Invalid job payload
- Unsupported job type
- Database unavailable
- Job not found
- Job handler failure
- Transaction failure
- Worker shutdown
- Invalid environment configuration

The application should not crash because one job fails.

### 16. Logging

Log useful events such as:

```text
job 42 created
worker 2 claimed job 42
job 42 completed
job 43 failed, retrying
job 43 permanently failed after 3 attempts
```

Do not build a complex logging framework for the MVP.

The standard `log` package is enough.

## Suggested Database Schema

Example:

```sql
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
```

Add indexes where useful, especially for selecting pending jobs.

Example:

```sql
CREATE INDEX idx_jobs_status_created_at
ON jobs(status, created_at);
```

## Suggested Project Structure

```text
go-job-queue/
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── config/
│   │   └── config.go
│   │
│   ├── database/
│   │   └── database.go
│   │
│   ├── jobs/
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── types.go
│   │
│   ├── worker/
│   │   ├── worker.go
│   │   └── pool.go
│   │
│   └── api/
│       └── handlers.go
│
├── db/
│   ├── migrations/
│   └── queries/
│
├── sqlc.yaml
├── docker-compose.yml
├── go.mod
├── README.md
└── REQUIREMENTS.md
```

Do not worry if the first version starts with fewer folders. Refactor as responsibilities become clearer.

## Suggested Implementation Order

- [ ] Initialize Go module
- [ ] Start PostgreSQL with Docker Compose
- [ ] Create jobs migration
- [ ] Configure `sqlc`
- [ ] Connect to PostgreSQL using `pgxpool`
- [ ] Implement `POST /jobs`
- [ ] Implement `GET /jobs/:id`
- [ ] Implement job claiming query
- [ ] Add `FOR UPDATE SKIP LOCKED`
- [ ] Implement one worker
- [ ] Expand to configurable worker pool
- [ ] Implement fake job handlers
- [ ] Mark successful jobs as `completed`
- [ ] Implement retries
- [ ] Implement permanent `failed` state
- [ ] Add worker polling delay
- [ ] Add `context.Context`
- [ ] Add graceful shutdown
- [ ] Add environment configuration
- [ ] Add basic logging
- [ ] Add tests
- [ ] Run `gofmt`
- [ ] Run `go vet`
- [ ] Write README

## MVP Completion Criteria

The MVP is complete when:

- [ ] A client can create a job through HTTP.
- [ ] Jobs are persisted in PostgreSQL.
- [ ] Multiple workers can process jobs concurrently.
- [ ] Two workers cannot claim the same job.
- [ ] Successful jobs become `completed`.
- [ ] Failed jobs are retried automatically.
- [ ] Jobs become permanently `failed` after reaching `max_attempts`.
- [ ] Jobs survive application restarts.
- [ ] Job status can be queried through HTTP.
- [ ] Workers stop cleanly during application shutdown.
- [ ] The application passes basic automated tests.

## Out of Scope

Do **not** implement yet:

- Multiple database engines
- MongoDB support
- Language-agnostic clients
- Redis
- Kafka
- RabbitMQ
- PostgreSQL `LISTEN/NOTIFY`
- Distributed workers running on multiple machines
- Scheduled jobs / cron jobs
- Delayed jobs
- Job priorities
- Job dependencies / workflows
- Dead-letter queues
- Web dashboard
- Authentication
- Multi-tenancy
- Horizontal scaling orchestration
- Kubernetes
- Advanced metrics / Prometheus
- Rate limiting

The focus of this project is:

> Build a reliable persistent worker queue using Go concurrency and PostgreSQL transactions.
