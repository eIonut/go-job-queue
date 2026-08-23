# Go Job Queue — MVP Requirements

## What This Project Actually Does

This project is a small standalone service that receives jobs, stores them in PostgreSQL, and processes them in the background using Go workers.

A **job** is simply a task that should be executed later or outside the main HTTP request.

For example, imagine another application needs to:

- send an email
- generate a PDF report
- resize an image
- process a video
- send a notification
- import a large CSV file

Instead of doing that work immediately inside the HTTP request, the application can create a job.

Example:

```text
User clicks "Generate report"
        |
        v
Main application
        |
        | POST /jobs
        v
Go Job Queue
        |
        v
PostgreSQL
```

The job queue responds quickly:

```json
{
  "id": 42,
  "status": "pending"
}
```

The actual report generation happens separately in the background.

---

## The Main Idea

Think about the project as having two main sides:

```text
                Go Job Queue Service

        HTTP API                Workers
           |                       |
           |                       |
           v                       v
        +-----------------------------+
        |         PostgreSQL          |
        |                             |
        | Job 1 - pending             |
        | Job 2 - processing          |
        | Job 3 - completed           |
        +-----------------------------+
```

The **HTTP API** creates jobs.

The **workers** execute jobs.

PostgreSQL sits between them and stores the queue permanently.

---

## What Is a Queue?

Imagine you have these jobs:

```text
Job 1
Job 2
Job 3
Job 4
Job 5
```

They are waiting to be processed.

You have three workers:

```text
Worker 1
Worker 2
Worker 3
```

Initially:

```text
Queue:

Job 1
Job 2
Job 3
Job 4
Job 5
```

The workers take jobs:

```text
Worker 1 -> Job 1
Worker 2 -> Job 2
Worker 3 -> Job 3
```

While those jobs are running:

```text
Queue:

Job 4
Job 5
```

When Worker 2 finishes Job 2:

```text
Worker 2 -> Job 4
```

When Worker 1 finishes Job 1:

```text
Worker 1 -> Job 5
```

So the workers continuously take available jobs until there are none left.

---

## Why Build a Job Queue?

Consider an HTTP endpoint:

```http
POST /reports
```

Generating the report takes 20 seconds.

Without a job queue:

```text
Request arrives
     |
     v
Generate report
     |
   20 sec
     |
     v
Return response
```

The user waits 20 seconds.

With a job queue:

```text
Request arrives
     |
     v
Create job
     |
     v
Return immediately
```

Then separately:

```text
Worker
   |
   v
Find pending job
   |
   v
Generate report
   |
   v
Mark completed
```

The expensive work no longer blocks the original HTTP request.

---

## What You Are Actually Building

You are essentially building a simplified version of systems such as:

```text
BullMQ
Sidekiq
Celery
Resque
Laravel Queues
```

But instead of using Redis or another message broker, your queue will use PostgreSQL.

The architecture will roughly be:

```text
                   CLIENT
                     |
                     |
                 POST /jobs
                     |
                     v
              +--------------+
              |   HTTP API   |
              +--------------+
                     |
                     v
              +--------------+
              |  PostgreSQL  |
              |              |
              | jobs table   |
              +--------------+
                 ^    ^    ^
                 |    |    |
                 |    |    |
             Worker Worker Worker
                1      2      3
```

Everything runs inside your Go application except PostgreSQL.

---

## Example From Start to Finish

Suppose the client sends:

```http
POST /jobs
```

with:

```json
{
  "type": "send_email",
  "payload": {
    "to": "user@example.com",
    "subject": "Welcome!"
  }
}
```

### Step 1 — API Receives the Job

Your HTTP handler receives the request.

It validates:

```text
type = send_email
payload = valid
```

Then inserts something like this into PostgreSQL:

```text
id: 42
type: send_email
payload: {...}
status: pending
attempts: 0
max_attempts: 3
```

Database:

```text
jobs

+----+------------+---------+----------+
| id | type       | status  | attempts |
+----+------------+---------+----------+
| 42 | send_email | pending | 0        |
+----+------------+---------+----------+
```

The API immediately returns:

```json
{
  "id": 42,
  "status": "pending"
}
```

---

## Step 2 — A Worker Finds the Job

One of your workers is running continuously.

Conceptually:

```go
for {
    findJob()
    processJob()
}
```

The worker asks PostgreSQL:

```text
Give me one pending job.
```

PostgreSQL returns:

```text
Job 42
```

The worker changes it to:

```text
status = processing
attempts = 1
```

Now the database contains:

```text
+----+------------+------------+----------+
| id | type       | status     | attempts |
+----+------------+------------+----------+
| 42 | send_email | processing | 1        |
+----+------------+------------+----------+
```

---

## Step 3 — The Worker Processes the Job

For the MVP, nothing real needs to happen.

You might have:

```go
func handleSendEmail(job Job) error {
    time.Sleep(2 * time.Second)

    return nil
}
```

This simulates sending an email.

Your worker looks at:

```text
job.type
```

and decides which handler to call.

Conceptually:

```go
switch job.Type {
case "send_email":
    handleSendEmail(job)

case "generate_report":
    handleGenerateReport(job)

case "resize_image":
    handleResizeImage(job)
}
```

---

## Step 4 — Job Succeeds

If:

```go
handleSendEmail(job)
```

returns:

```go
nil
```

the worker updates PostgreSQL:

```text
status = completed
```

Database:

```text
+----+------------+-----------+----------+
| id | type       | status    | attempts |
+----+------------+-----------+----------+
| 42 | send_email | completed | 1        |
+----+------------+-----------+----------+
```

The job is finished.

---

## What Happens If the Job Fails?

Imagine:

```go
func handleSendEmail(job Job) error {
    return errors.New("email server unavailable")
}
```

The job was:

```text
attempts = 1
max_attempts = 3
```

Because:

```text
1 < 3
```

the worker changes it back to:

```text
pending
```

So:

```text
processing
    |
    | error
    v
pending
```

A worker can later pick it up again.

---

## Retry Example

Imagine this job:

```text
max_attempts = 3
```

First attempt:

```text
attempt 1

pending
   |
   v
processing
   |
   | ERROR
   v
pending
```

Second attempt:

```text
attempt 2

pending
   |
   v
processing
   |
   | ERROR
   v
pending
```

Third attempt:

```text
attempt 3

pending
   |
   v
processing
   |
   | ERROR
   v
failed
```

Now the system stops retrying the job.

---

## Why PostgreSQL?

PostgreSQL isn't just storing history.

For this MVP, PostgreSQL **is the queue**.

If you have:

```text
Job 1 - pending
Job 2 - pending
Job 3 - pending
Job 4 - completed
Job 5 - failed
```

the workers search for:

```text
status = pending
```

Therefore:

```text
PostgreSQL table
      =
job queue + persistent storage
```

This also means that if your Go application crashes, the jobs are still there.

Example:

```text
Application running

Job 42 -> pending
Job 43 -> pending

        CRASH
          X

PostgreSQL still contains:

Job 42 -> pending
Job 43 -> pending
```

When the application starts again:

```text
Worker starts
     |
     v
find pending jobs
     |
     v
Job 42
```

This is what makes the queue persistent.

---

## Why Multiple Workers?

Suppose processing one job takes 2 seconds.

With one worker:

```text
Worker 1

Job 1 -> 2 sec
Job 2 -> 2 sec
Job 3 -> 2 sec
Job 4 -> 2 sec
Job 5 -> 2 sec
```

Approximately:

```text
10 seconds
```

With five workers:

```text
Worker 1 -> Job 1
Worker 2 -> Job 2
Worker 3 -> Job 3
Worker 4 -> Job 4
Worker 5 -> Job 5
```

All jobs can run concurrently.

Approximately:

```text
2 seconds
```

This is where Go's goroutines become useful.

---

## The Worker Pool

If:

```env
WORKERS=5
```

your application starts five goroutines.

Conceptually:

```go
for i := 0; i < 5; i++ {
    go worker.Start()
}
```

So you have:

```text
Go process

├── HTTP server
│
├── Worker goroutine 1
├── Worker goroutine 2
├── Worker goroutine 3
├── Worker goroutine 4
└── Worker goroutine 5
```

They all use the same PostgreSQL jobs table.

---

## The Main Concurrency Problem

Imagine PostgreSQL contains:

```text
Job 42 - pending
```

And simultaneously:

```text
Worker 1 asks for a job
Worker 2 asks for a job
```

Without protection, both could receive:

```text
Job 42
```

Then you could accidentally send the same email twice.

```text
Worker 1 -> Job 42 -> send email

Worker 2 -> Job 42 -> send email
```

This is a serious bug.

---

## Why `FOR UPDATE SKIP LOCKED` Exists

This is where PostgreSQL row locking comes in.

Worker 1 starts a transaction:

```text
Worker 1
   |
   v
locks Job 42
```

Worker 2 searches for a job.

It sees:

```text
Job 42 = locked
```

Because you use:

```sql
FOR UPDATE SKIP LOCKED
```

Worker 2 does not wait for Job 42.

It skips it:

```text
Worker 1 -> Job 42
Worker 2 -> Job 43
Worker 3 -> Job 44
```

This is one of the most important parts of the project.

---

## What Is the HTTP API For?

Your job queue is a standalone service.

Other applications can communicate with it using HTTP.

For example:

```text
React app
Node backend
Go backend
Python backend
Mobile backend
       |
       |
       v
POST http://job-queue:8080/jobs
```

The queue doesn't care which application created the request.

It just receives:

```json
{
  "type": "generate_report",
  "payload": {
    "user_id": 123
  }
}
```

and stores it.

---

## Checking a Job

After creating:

```text
Job 42
```

a client can ask:

```http
GET /jobs/42
```

While waiting:

```json
{
  "id": 42,
  "status": "processing"
}
```

Later:

```json
{
  "id": 42,
  "status": "completed"
}
```

So the caller does not need to keep the original HTTP connection open.

---

## What Runs Inside the Application?

Conceptually your final application will look like:

```text
main.go
   |
   +-------------------------+
   |                         |
   v                         v
HTTP Server             Worker Pool
   |                         |
   |                         |
POST /jobs               Worker 1
GET /jobs/:id            Worker 2
                         Worker 3
                         Worker 4
                         Worker 5
   |                         |
   +-----------+-------------+
               |
               v
          PostgreSQL
```

---

## Where Channels Fit

Channels are useful for communication between goroutines inside the Go application.

However, PostgreSQL remains the persistent queue.

You should not make the entire queue depend on an in-memory channel such as:

```go
jobs := make(chan Job)
```

because if the application crashes, everything inside that channel disappears.

Instead:

```text
PostgreSQL
    |
    v
Worker goroutines
```

is the source of truth.

Channels can still be useful internally for coordination, worker lifecycle, shutdown, or passing results between goroutines.

---

## Where `context.Context` Fits

Your application will have a main context.

Conceptually:

```text
context
   |
   +---- HTTP server
   |
   +---- Worker 1
   |
   +---- Worker 2
   |
   +---- Worker 3
```

Normally:

```text
ctx active
```

so workers continue running.

When the application receives:

```text
Ctrl+C
```

you cancel the context:

```text
ctx cancelled
```

Workers can detect that:

```go
select {
case <-ctx.Done():
    return
default:
    // continue working
}
```

So the context is essentially the signal saying:

```text
The application is shutting down.
Stop your work when appropriate.
```

---

## What Graceful Shutdown Means

Imagine Worker 1 is currently processing:

```text
Job 42
```

Then you press:

```text
Ctrl+C
```

You don't want:

```text
Ctrl+C
   |
   v
kill everything instantly
```

Instead:

```text
Ctrl+C
   |
   v
stop accepting new requests
   |
   v
tell workers to stop
   |
   v
finish current work if possible
   |
   v
close PostgreSQL pool
   |
   v
exit
```

That is graceful shutdown.

---

## What You Will Have Built at the End

You will be able to start:

```bash
go run ./cmd/server
```

and your program will run something conceptually like:

```text
2026/08/21 server listening on :8080
2026/08/21 started worker 1
2026/08/21 started worker 2
2026/08/21 started worker 3
```

Then:

```bash
curl -X POST localhost:8080/jobs ...
```

might produce:

```text
job 42 created
```

Then:

```text
worker 2 claimed job 42
job 42 processing
job 42 completed
```

And PostgreSQL will contain the complete state of that job.

---

## Mental Model

The easiest way to think about the whole project is:

```text
                "I have work to do later"
                         |
                         v
                    POST /jobs
                         |
                         v
                 Save in PostgreSQL
                         |
                         v
                      pending
                         |
                         v
                Worker finds the job
                         |
                         v
                    processing
                    /         \
                   /           \
               success         error
                  |               |
                  v               v
             completed          retry
                                 |
                                 v
                              pending
                                 |
                              or after
                           too many attempts
                                 |
                                 v
                               failed
```

The main problem this project solves is:

> **How can I reliably execute background work concurrently without losing tasks, processing the same task twice, or blocking the application that created the task?**

---

# Goal

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

> _Build a reliable persistent worker queue using Go concurrency and PostgreSQL transactions._
