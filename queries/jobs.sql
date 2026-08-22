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