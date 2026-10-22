-- Runs once when the pro_manage Postgres volume is first created.
-- pm_core is created by POSTGRES_DB; Keycloak gets its own database on the same instance in dev.
CREATE DATABASE keycloak;
