-- name: InsertMembership :exec
INSERT INTO memberships (user_id, project_id, role_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE user_id = $1 AND project_id = $2 AND role_id = $3;

-- name: ListProjectMembers :many
SELECT u.id AS user_id, u.github_login, u.display_name,
       COALESCE(u.avatar_url, '')::text AS avatar_url, u.is_admin,
       r.id AS role_id, r.key AS role_key, r.label AS role_label,
       COALESCE(r.color, '')::text AS role_color
FROM memberships m
JOIN users u ON u.id = m.user_id
JOIN roles r ON r.id = m.role_id
WHERE m.project_id = $1
ORDER BY u.display_name, r.label;

-- name: ListMembershipsForUser :many
SELECT p.id AS project_id, p.slug AS project_slug, p.name AS project_name,
       r.id AS role_id, r.key AS role_key, r.label AS role_label,
       COALESCE(r.color, '')::text AS role_color, r.position AS role_position
FROM memberships m
JOIN projects p ON p.id = m.project_id
JOIN roles    r ON r.id = m.role_id
WHERE m.user_id = $1
ORDER BY p.slug, r.position, r.label;

-- name: ListUserRoleIDsInProject :many
SELECT role_id FROM memberships
WHERE user_id = $1 AND project_id = $2;
