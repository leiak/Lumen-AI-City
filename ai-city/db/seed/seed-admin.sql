-- ============================================
-- Phase C.4 — Admin seed for BT editor (mount slot 06)
-- ============================================
-- Seed an admin / adminpass player with role='admin' so that
-- admin-portal's /login form can authenticate against PG.
--
-- The schema migration ``pg-schema-2.0-bt-auth.sql`` adds the role
-- column; this seed depends on it (must run AFTER 05-bt-auth.sql in
-- the docker-entrypoint-initdb.d mount order).
--
-- Idempotent: ON CONFLICT updates password_hash + role, so re-running
-- after a password rotation picks up the new credential.
--
-- Password: adminpass (bcrypt cost 10, matches pg-schema.sql demo seed)
-- Production override: set ADMIN_USERNAME / ADMIN_PASSWORD env vars in
-- admin-portal — the seed here is the dev default.

INSERT INTO player (username, email, password_hash, display_name, role)
VALUES (
    'admin',
    'admin@aicity.dev',
    crypt('adminpass', gen_salt('bf', 10)),
    'Admin',
    'admin'
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    role          = EXCLUDED.role;
