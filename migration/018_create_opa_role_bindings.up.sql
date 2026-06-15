CREATE TABLE opa_role_bindings (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    subject    VARCHAR(255) NOT NULL,
    role       VARCHAR(50)  NOT NULL,
    domain     VARCHAR(500) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    UNIQUE (subject, role, domain)
);

CREATE INDEX idx_opa_role_bindings_subject ON opa_role_bindings(subject);
