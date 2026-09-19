#!/bin/bash
#
# Creates the login of the application role, or updates its password.
# Runs automatically on the first start of a fresh volume. For an existing volume:
#   make db_app_role

set -euo pipefail

psql -v ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  -v app_password="${APP_DB_PASSWORD:-jungle_app}" <<'SQL'
SELECT format('CREATE ROLE jungle_app LOGIN PASSWORD %L', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'jungle_app')
\gexec

SELECT format('ALTER ROLE jungle_app LOGIN PASSWORD %L', :'app_password')
\gexec
SQL
