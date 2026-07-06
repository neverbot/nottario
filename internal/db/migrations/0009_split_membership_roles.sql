-- +goose Up
-- +goose StatementBegin
-- Split the old `memberships (user_id, project_id, role_id)` table
-- into two, along the natural axis:
--   memberships (user_id, project_id)                     — "X is in P"
--   membership_roles (user_id, project_id, role_id)       — "X holds R in P"
-- The old FK `memberships.role_id → roles(id) ON DELETE CASCADE`
-- meant that deleting a role wiped every (user, project) row that
-- referenced it, so a user with only that role dropped out of the
-- project. See feature task 663b7219 for the full context.
--
-- After this migration:
--   * Deleting a role cascades to membership_roles (loses the assignment)
--     but leaves memberships untouched — the member stays in the project.
--   * A member with zero role assignments is a legal state.
--   * Access checks that read `EXISTS memberships WHERE user_id AND project_id`
--     remain valid (the presence in memberships still denotes access).
CREATE TABLE membership_roles (
    user_id    uuid NOT NULL,
    project_id uuid NOT NULL,
    role_id    uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id, role_id),
    CONSTRAINT membership_roles_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT membership_roles_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT membership_roles_role_id_fkey
        FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

CREATE INDEX membership_roles_project_role_idx
    ON membership_roles (project_id, role_id);
CREATE INDEX membership_roles_role_id_idx
    ON membership_roles (role_id);

-- Copy every existing (user, project, role) triple across.
INSERT INTO membership_roles (user_id, project_id, role_id, created_at)
SELECT user_id, project_id, role_id, created_at FROM memberships;

-- Rebuild memberships with the new (user_id, project_id) PK. We use a
-- staging table so the reshape is atomic within this migration.
CREATE TABLE memberships_new (
    user_id    uuid NOT NULL,
    project_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id),
    CONSTRAINT memberships_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT memberships_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

INSERT INTO memberships_new (user_id, project_id, created_at)
SELECT user_id, project_id, MIN(created_at)
  FROM memberships
 GROUP BY user_id, project_id;

DROP TABLE memberships;
ALTER TABLE memberships_new RENAME TO memberships;
CREATE INDEX memberships_project_idx ON memberships (project_id);

-- Composite FK from membership_roles into the new memberships PK so
-- removing a member cascades cleanly to all their role assignments.
ALTER TABLE membership_roles
    ADD CONSTRAINT membership_roles_membership_fkey
        FOREIGN KEY (user_id, project_id)
        REFERENCES memberships(user_id, project_id)
        ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Reverse the split. Rows that represented a "member with no roles"
-- under the split cannot be expressed in the old shape, so those
-- members drop out. This is inherent — the old shape conflated the
-- two axes.
CREATE TABLE memberships_old (
    user_id    uuid NOT NULL,
    project_id uuid NOT NULL,
    role_id    uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id, role_id),
    CONSTRAINT memberships_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT memberships_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT memberships_role_id_fkey
        FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

INSERT INTO memberships_old (user_id, project_id, role_id, created_at)
SELECT mr.user_id, mr.project_id, mr.role_id, mr.created_at
  FROM membership_roles mr;

DROP TABLE membership_roles;
DROP TABLE memberships;
ALTER TABLE memberships_old RENAME TO memberships;
CREATE INDEX memberships_project_idx ON memberships (project_id);
-- +goose StatementEnd
