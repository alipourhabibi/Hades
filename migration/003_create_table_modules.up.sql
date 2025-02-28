-- Create the modules table
CREATE TABLE IF NOT EXISTS modules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    create_time TIMESTAMP NOT NULL DEFAULT NOW(),
    update_time TIMESTAMP NOT NULL DEFAULT NOW(),
    name VARCHAR(255) UNIQUE NOT NULL,
    owner_id UUID NOT NULL,
    visibility SMALLINT NOT NULL DEFAULT 1,  -- Visibility using the ModuleVisibility enum
    state SMALLINT NOT NULL DEFAULT 1,       -- State using the ModuleState enum
    description TEXT,
    url TEXT UNIQUE,
    default_label_name VARCHAR(20) DEFAULT 'main',
    default_branch VARCHAR(20) DEFAULT 'main',

    -- Foreign Key Constraints
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX idx_modules_owner_id ON modules(owner_id);
CREATE INDEX idx_modules_url ON modules(url);
