-- name: CreateJob :one
INSERT INTO jobs (
    type,
    payload
)
VALUES (
    $1,
    $2
)
RETURNING *;

-- name: GetJob :one
SELECT *
FROM jobs
WHERE id = $1;

-- name: GetJobs :many
SELECT * 
FROM jobs
ORDER BY created_at DESC;

-- name: GetPendingJob :one
SELECT * 
FROM jobs
WHERE status = 'pending'
ORDER BY created_at
LIMIT 1;

-- name: MarkJobRunning :exec
UPDATE jobs
SET status = 'running', updated_at = NOW()
WHERE id = $1;

-- name: MarkJobCompleted :exec
UPDATE jobs
SET status = 'completed', updated_at = NOW(), retry_at = NULL
where id = $1;

-- name: MarkJobFailed :exec
UPDATE jobs
SET status = 'failed', attempts = attempts + 1,  updated_at = NOW()
where id = $1;

-- name: ScheduleRetry :exec
UPDATE jobs
SET status = 'pending', 
    retry_at = NOW() + INTERVAL '10 seconds',
    updated_at = NOW()
WHERE id = $1;

-- name: ClaimPendingJob :one
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
SET status = 'running', updated_at = NOW()
WHERE id = (SELECT id FROM next_job)
RETURNING *;