-- Optional password login for users created by workspace admins.
-- Kept off the "user" table so existing SELECT * user queries stay stable.
-- Application code owns the user_id relationship; no FK / cascade.
CREATE TABLE IF NOT EXISTS user_password (
    user_id UUID PRIMARY KEY,
    password_hash TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
