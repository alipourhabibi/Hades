CREATE TABLE org_memberships (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  member_id  UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       TEXT        NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, member_id)
);

CREATE INDEX org_memberships_org_id_idx    ON org_memberships (org_id);
CREATE INDEX org_memberships_member_id_idx ON org_memberships (member_id);
