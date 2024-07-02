CREATE TABLE IF NOT EXISTS admin_users (
 "id" integer not null primary key,
 "source" text,
 "users" text,
 "last_updated_at" text
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_source ON admin_users (source);
