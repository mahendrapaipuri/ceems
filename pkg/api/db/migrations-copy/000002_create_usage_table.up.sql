CREATE TABLE IF NOT EXISTS usage (
 "id" integer not null primary key,
 "num_units" integer,
 "project" text,
 "usr" text,
 "total_cpu_billing" integer,
 "total_gpu_billing" integer,
 "total_misc_billing" integer,
 "avg_cpu_usage" real,
 "avg_cpu_mem_usage" real,
 "total_cpu_energy_usage_kwh" real,
 "total_cpu_emissions_gms" real,
 "avg_gpu_usage" real,
 "avg_gpu_mem_usage" real,
 "total_gpu_energy_usage_kwh" real,
 "total_gpu_emissions_gms" real,
 "total_io_write_hot_gb" real,
 "total_io_read_hot_gb" real,
 "total_io_write_cold_gb" real,
 "total_io_read_cold_gb" real,
 "total_ingress_in_gb" real,
 "total_outgress_in_gb" real
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_usr ON usage (usr,project);
