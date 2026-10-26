-- +goose Up
-- Project membership and section-level ACL grants (FSD §03 ProjectMember, ACLGrant; §04 base roles,
-- sections, grant evaluation). user_id is the login handle, like every other actor column today (B-11).

CREATE TABLE project_members (
    project_id   uuid NOT NULL REFERENCES projects (project_id),
    user_id      text NOT NULL,
    base_role    text NOT NULL CHECK (base_role IN ('owner', 'editor', 'viewer', 'contributor')),
    invited_by   text NOT NULL,
    invited_at   timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz,
    expires_at   timestamptz,                                  -- time-boxed access (auditors, contractors)
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_members_user_idx ON project_members (user_id);

CREATE TABLE acl_grants (
    grant_id     uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects (project_id),
    user_id      text NOT NULL,
    scope_kind   text NOT NULL CHECK (scope_kind IN ('section', 'document', 'field')),
    scope_target text NOT NULL,                                 -- section:budget | document:<id> | field:<path>
    permission   text NOT NULL CHECK (permission IN ('read', 'write', 'read_full', 'deny')),
    granted_by   text NOT NULL,
    granted_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz,
    reason       text NOT NULL DEFAULT '',
    UNIQUE (project_id, user_id, scope_kind, scope_target, permission)
);
CREATE INDEX acl_grants_project_user_idx ON acl_grants (project_id, user_id);

-- Every existing project's owner is its owner member (the spine's projects were created before membership existed).
INSERT INTO project_members (project_id, user_id, base_role, invited_by, activated_at)
SELECT project_id, owner_user_id, 'owner', created_by, created_at FROM projects
ON CONFLICT (project_id, user_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS acl_grants;
DROP TABLE IF EXISTS project_members;
