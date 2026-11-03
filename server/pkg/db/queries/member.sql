-- name: ListMembers :many
SELECT * FROM member
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: ListWorkspaceManagerUserIDs :many
SELECT user_id FROM member
WHERE workspace_id = $1 AND role IN ('owner', 'admin')
ORDER BY created_at ASC;

-- name: GetMember :one
SELECT * FROM member
WHERE id = $1;

-- name: GetMemberByUserAndWorkspace :one
SELECT * FROM member
WHERE user_id = $1 AND workspace_id = $2;

-- name: GetRealtimeMemberships :many
SELECT m.user_id, m.workspace_id
FROM member m
JOIN (
    SELECT unnest(sqlc.arg(user_ids)::uuid[]) AS user_id,
           unnest(sqlc.arg(workspace_ids)::uuid[]) AS workspace_id
) AS requested
    ON m.user_id = requested.user_id AND m.workspace_id = requested.workspace_id;

-- name: CreateMember :one
INSERT INTO member (workspace_id, user_id, role, remark)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateMemberRole :one
UPDATE member SET role = $2
WHERE id = $1
RETURNING *;

-- name: UpdateMemberRemark :one
UPDATE member SET remark = $2
WHERE id = $1
RETURNING *;

-- name: UpdateMemberRoleAndRemark :one
UPDATE member SET
    role = COALESCE(sqlc.narg(role), role),
    remark = COALESCE(sqlc.narg(remark), remark)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteMember :exec
DELETE FROM member WHERE id = $1;

-- name: ListMembersWithUser :many
SELECT m.id, m.workspace_id, m.user_id, m.role, m.remark, m.created_at,
       u.name as user_name, u.email as user_email, u.avatar_url as user_avatar_url,
       EXISTS(
           SELECT 1 FROM user_password up
           WHERE up.user_id = u.id AND up.password_hash <> ''
       ) AS has_password
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = $1
ORDER BY m.created_at ASC;
