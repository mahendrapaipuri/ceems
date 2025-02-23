ALTER TABLE admin_users RENAME TO tmp_admin_users;
CREATE TABLE IF NOT EXISTS admin_users (
 "id" integer not null primary key,
 "source" text,
 "users" text default '[]',
 "last_updated_at" text
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_source ON admin_users (source);
INSERT INTO admin_users (source,users,last_updated_at) SELECT value, json_group_array(name),last_updated_at FROM tmp_admin_users, json_each(tags) WHERE name != '';
DROP TABLE IF EXISTS tmp_admin_users;
