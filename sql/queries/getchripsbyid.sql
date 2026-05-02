-- name: GetChripsID :one
SELECT * FROM chirps
WHERE id = $1;