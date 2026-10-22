-- name: CreateSite :one
INSERT INTO sites (
    site_id, tenant_id, code, name, portfolio_id, country, time_zone, currency, unit_system,
    business_calendar_id, regulatory_regime, privacy_regime, address, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14
) RETURNING *;

-- name: GetSite :one
SELECT * FROM sites WHERE tenant_id = $1 AND site_id = $2;

-- name: GetSiteByCode :one
SELECT * FROM sites WHERE tenant_id = $1 AND code = $2;

-- name: ListSites :many
SELECT * FROM sites WHERE tenant_id = $1 ORDER BY code;
