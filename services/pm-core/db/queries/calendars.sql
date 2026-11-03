-- Business calendars (FSD §03, §08; backlog B-07).

-- name: GetBusinessCalendar :one
SELECT * FROM business_calendars WHERE tenant_id = $1 AND calendar_id = $2;

-- name: GetCalendarForSite :one
-- The site's calendar, or no row when the site has none (callers fall back to the wall clock).
SELECT c.* FROM sites s JOIN business_calendars c ON c.calendar_id = s.business_calendar_id
WHERE s.tenant_id = $1 AND s.site_id = $2;

-- name: GetCalendarForProject :one
SELECT c.* FROM projects p JOIN sites s ON s.site_id = p.site_id JOIN business_calendars c ON c.calendar_id = s.business_calendar_id
WHERE p.tenant_id = $1 AND p.project_id = $2;

-- name: UpsertBusinessCalendar :one
INSERT INTO business_calendars (calendar_id, tenant_id, name, time_zone, weekend_days, holidays)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (calendar_id) DO UPDATE SET name = EXCLUDED.name, time_zone = EXCLUDED.time_zone, weekend_days = EXCLUDED.weekend_days, holidays = EXCLUDED.holidays, updated_at = now()
RETURNING *;

-- name: SetSiteCalendar :exec
UPDATE sites SET business_calendar_id = $3, updated_at = now(), updated_by = $4, version = version + 1 WHERE tenant_id = $1 AND site_id = $2;
