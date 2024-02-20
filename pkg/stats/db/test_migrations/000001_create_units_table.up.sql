CREATE TABLE IF NOT EXISTS units (
 "id" integer not null primary key,
 "uuid" text,
 "name" text,
 "project" text,
 "grp" text,
);
CREATE INDEX IF NOT EXISTS idx_usr_project_start ON units (usr,project,start);
