-- +goose Up
-- Business calendars (FSD §03 Site "business_calendar_id", §08 "SLA clocks run in business days of
-- the project's site calendar"; backlog B-07). A calendar names the weekend days (ISO numbering,
-- 1 Monday to 7 Sunday) and the holiday dates of a site, in the site's time zone. Stage SLA clocks
-- run only through business days of the project's site; a site without a calendar keeps the wall
-- clock, which is what every stage used before this migration.
CREATE TABLE business_calendars (
    calendar_id  text PRIMARY KEY,
    tenant_id    text NOT NULL DEFAULT 'default',
    name         text NOT NULL,
    time_zone    text NOT NULL,                                 -- IANA; the day boundaries of the calendar
    weekend_days smallint[] NOT NULL DEFAULT '{6,7}',           -- ISO weekday numbers never worked
    holidays     date[] NOT NULL DEFAULT '{}',                  -- dates never worked
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE sites ADD CONSTRAINT sites_business_calendar_fkey FOREIGN KEY (business_calendar_id) REFERENCES business_calendars (calendar_id);

-- +goose Down
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_business_calendar_fkey;
DROP TABLE IF EXISTS business_calendars;
