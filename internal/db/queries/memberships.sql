-- name: EnsureMembership :exec
-- Idempotent: marks the user as a member of the project. Role
-- assignments live in a separate table so a membership without any
-- roles is a legal state.
INSERT INTO memberships (user_id, project_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveMembership :exec
-- Removes the user from the project entirely; their role assignments
-- cascade away via membership_roles_membership_fkey.
DELETE FROM memberships
WHERE user_id = $1 AND project_id = $2;

-- name: AssignRole :exec
-- Grants a specific role to a member. Idempotent.
INSERT INTO membership_roles (user_id, project_id, role_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: UnassignRole :exec
-- Revokes a specific role from a member. The membership row itself
-- stays — dropping the last role does NOT remove the member from
-- the project.
DELETE FROM membership_roles
WHERE user_id = $1 AND project_id = $2 AND role_id = $3;

-- name: ListProjectMembers :many
-- One row per (member, role). A member with no role assignments
-- still appears once, with role columns NULL, via LEFT JOIN.
SELECT u.id AS user_id, u.github_login, u.display_name,
       COALESCE(u.avatar_url, '')::text AS avatar_url, u.is_admin,
       mr.role_id AS role_id,
       COALESCE(r.key, '')::text     AS role_key,
       COALESCE(r.label, '')::text   AS role_label,
       COALESCE(r.color, '')::text   AS role_color
FROM memberships m
JOIN users u ON u.id = m.user_id
LEFT JOIN membership_roles mr
       ON mr.user_id = m.user_id AND mr.project_id = m.project_id
LEFT JOIN roles r ON r.id = mr.role_id
WHERE m.project_id = $1
ORDER BY u.display_name, r.position NULLS LAST, r.label;

-- name: ListMembershipsForUser :many
-- One row per (project, role) the user holds. Projects where the
-- user has zero roles are omitted here on purpose — this feeds the
-- role-scoped whoami view; use ListProjectIDsForUser to enumerate
-- project membership regardless of roles.
SELECT p.id AS project_id, p.slug AS project_slug, p.name AS project_name,
       mr.role_id, r.key AS role_key, r.label AS role_label,
       COALESCE(r.color, '')::text AS role_color, r.position AS role_position
FROM memberships m
JOIN projects p ON p.id = m.project_id
JOIN membership_roles mr
     ON mr.user_id = m.user_id AND mr.project_id = m.project_id
JOIN roles r ON r.id = mr.role_id
WHERE m.user_id = $1
ORDER BY p.slug, r.position, r.label;

-- name: ListUserRoleIDsInProject :many
SELECT role_id FROM membership_roles
WHERE user_id = $1 AND project_id = $2;
