-- More ways to sign in, and to get back in (internal/account): Sign in with
-- Google, confirming email addresses, resetting a forgotten password, the
-- Android app's sign-in, and demo accounts.

-- google_sub is Google's stable ID for the person; it stays NULL until they
-- first sign in with Google. People who only ever sign in with Google have
-- an empty password_hash, which no password matches.
ALTER TABLE users ADD COLUMN google_sub TEXT UNIQUE;

-- Set once the person has shown they receive mail at the address: by
-- opening the link we sent, or by signing in with Google.
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;

-- Demo accounts: "Try the demo" makes each visitor a throwaway account full
-- of sample data, with no password. It ends at demo_expires_at, after which
-- it is deleted with everything in it. NULL for everyone else.
ALTER TABLE users ADD COLUMN demo_expires_at TIMESTAMPTZ;
CREATE INDEX users_demo_expires_at_idx ON users (demo_expires_at) WHERE demo_expires_at IS NOT NULL;

-- Links sent by email to confirm an address. Only a SHA-256 hash of each
-- token is stored.
CREATE TABLE email_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL, -- the address the link was sent to
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_user_id_idx ON email_tokens (user_id);

-- Resetting a forgotten password: a link sent by email, single use, for an
-- hour. Only a SHA-256 hash of each token is stored, and a link stops
-- working if the account's email changes.
CREATE TABLE password_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL, -- the address the link was sent to
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_tokens_user_id_idx ON password_tokens (user_id);

-- Signing in to an Android app with Google (internal/account/app.go). The
-- sign-in happens in the phone's browser, which hands the app a one-time
-- code; the app trades it, with the secret whose hash (challenge) it sent
-- first, for a session token. Only SHA-256 hashes of codes are stored.
CREATE TABLE app_codes (
    code_hash  BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    challenge  BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX app_codes_user_id_idx ON app_codes (user_id);
