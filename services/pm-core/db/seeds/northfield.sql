-- Development seed: the fictional Northfield Spirits Co. (FSD §05 local dev; decided 2026-10-20).
-- Idempotent: safe to run repeatedly. Never used outside dev and CI.
-- Apply with `make db-seed`.

INSERT INTO sites (site_id, code, name, country, time_zone, currency, unit_system, regulatory_regime, privacy_regime, address, created_by, updated_by) VALUES
  ('01900000-0000-7000-8000-00000000a001', 'NF-KY', 'Northfield Distillery, Kentucky', 'US', 'America/New_York', 'USD', 'imperial', '{ttb,osha,nfpa_30}', '{state_privacy}', '{"city":"Bardstown-ish (fictional)","state":"KY"}', 'seed', 'seed'),
  ('01900000-0000-7000-8000-00000000a002', 'NF-ON', 'Northfield Bottling, Ontario',    'CA', 'America/Toronto',  'CAD', 'metric',   '{cfia,excise_ca,provincial_ohs}', '{pipeda}', '{"city":"Niagara-ish (fictional)","province":"ON"}', 'seed', 'seed')
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO projects (project_id, code, name, description, site_id, currency, owner_user_id, lifecycle_state, fel_stage, project_type, tpm_pillar_tags, hazardous_area_classification, regulatory_stamps, onboarding_level, start_date, target_end_date, baseline_budget_total, baseline_duration_days, tags, created_by, updated_by) VALUES
  ('01900000-0000-7000-8000-00000000b001', 'P-NF-KY-7',  'Still House 2 column still',          'Greenfield continuous column still and beer well; 1.2M proof gallons per year.', '01900000-0000-7000-8000-00000000a001', 'USD', 'alice.owner', 'plan',     'fel3', 'greenfield',            '{eem}',                       'class_i_div_1', '{ttb,nfpa_30}', 'full',       '2026-09-01', '2028-03-31', 42000000.00, 942, '{still,capacity}', 'seed', 'seed'),
  ('01900000-0000-7000-8000-00000000b002', 'P-NF-KY-12', 'Bottling Line 3 changeover reduction', 'Tooling carts, quick-change guides, and recipe automation to cut SKU changeover on Line 3.', '01900000-0000-7000-8000-00000000a001', 'USD', 'alice.owner', 'execute',  'n/a',  'efficiency',            '{focused_improvement,eem}',   'none',          '{osha}',        'full',       '2026-04-15', '2026-12-20', 1850000.00, 249, '{bottling,changeover}', 'seed', 'seed'),
  ('01900000-0000-7000-8000-00000000b003', 'P-NF-KY-15', 'Boiler 2 burner replacement',         'Reliability replacement of the Boiler 2 burner and controls after repeat trips.', '01900000-0000-7000-8000-00000000a001', 'USD', 'alice.owner', 'initiate', 'fel1', 'reliability',           '{planned_maintenance,eem}',   'none',          '{nfpa_85}',     'governed',   NULL, NULL, 640000.00, NULL, '{boiler}', 'seed', 'seed'),
  ('01900000-0000-7000-8000-00000000b004', 'P-NF-ON-3',  'Dust collection upgrade, dry goods',  'Compliance upgrade of the dry-goods dust collection system.', '01900000-0000-7000-8000-00000000a002', 'CAD', 'alice.owner', 'plan',     'fel2', 'compliance',            '{she,eem}',                   'class_ii_div_1','{cfia,provincial_ohs,nfpa_652}', 'governed', '2026-10-01', '2027-05-30', 980000.00, 241, '{dust,compliance}', 'seed', 'seed'),
  ('01900000-0000-7000-8000-00000000b005', 'P-NF-ON-5',  'Case packer refurbishment',           'Refurbish case packer CP-2; small project, registered only.', '01900000-0000-7000-8000-00000000a002', 'CAD', 'alice.owner', 'execute',  'n/a',  'reliability',           '{planned_maintenance}',       'none',          '{}',            'registered', '2026-08-01', '2026-11-15', 60000.00, 106, '{packaging}', 'seed', 'seed')
ON CONFLICT (tenant_id, code) DO NOTHING;

-- Starter policy variables (FSD §08 "Starter variables"; placeholder defaults until the DoA is loaded, OI-14).
INSERT INTO policy_variables (key, type, unit, description, global_default, allowed_range, overridable_at, owner_role, created_by, updated_by) VALUES
  ('car.threshold.finance_approver', 'money', 'USD', 'CAR stage 2 (finance approver) applies at or above this amount', '0', '{"min":0}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('car.threshold.vp_finance', 'money', 'USD', 'CAR stage 3 (VP Finance) and MDEMG Tier A apply at or above this amount', '5000000', '{"min":0}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('car.threshold.cfo', 'money', 'USD', 'CAR stage 4 (CFO) applies at or above this amount', '25000000', '{"min":0}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('car.threshold.ceo', 'money', 'USD', 'CAR stage 5 (CEO) applies at or above this amount', '100000000', '{"min":0}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('car.threshold.basis', 'enum', NULL, 'What the CAR amount is compared on', '"tic"', '{"values":["tic","tic_plus_contingency","total_project_cost"]}', '{portfolio,site}', 'policy_owner', 'seed', 'seed'),
  ('car.threshold.currency', 'enum', NULL, 'Currency the CAR thresholds are expressed in', '"USD"', '{"values":["USD","CAD"]}', '{}', 'policy_owner', 'seed', 'seed'),
  ('car.supplemental.reapproval_pct', 'percent', 'pct', 'Cost growth on an approved CAR that forces a supplemental', '10', '{"min":0,"max":100}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('car.esign.required_above', 'money', 'USD', 'DocuSign required at or above this amount; internal attestation below', '1000000', '{"min":0}', '{portfolio,site}', 'policy_owner', 'seed', 'seed'),
  ('baseline.variance.reforecast_pct', 'percent', 'pct', 'Variance that triggers a Jiminy warning and a monthly re-forecast task', '10', '{"min":0,"max":100}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('baseline.variance.finance_pct', 'percent', 'pct', 'Baseline change stage 2 applies above this variance', '5', '{"min":0,"max":100}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('baseline.variance.vp_finance_pct', 'percent', 'pct', 'Baseline change stage 3 and MDEMG Tier A apply above this variance', '10', '{"min":0,"max":100}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('health_review.cadence', 'duration', 'days', 'Project health review cadence', '30', '{"min":7,"max":90}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('health_review.high_value_threshold', 'money', 'USD', 'Biweekly health reviews at or above this amount', '25000000', '{"min":0}', '{portfolio,site}', 'policy_owner', 'seed', 'seed'),
  ('early_mgmt.exit.nameplate_pct', 'percent', 'pct', 'Startup curve exit criterion: sustained share of nameplate', '95', '{"min":50,"max":100}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('early_mgmt.exit.sustain_days', 'integer', 'days', 'Startup curve exit criterion: days sustained', '14', '{"min":1,"max":90}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('benefits.window_days', 'integer', 'days', 'Benefits Realization window', '365', '{"min":90,"max":730}', '{project_type,portfolio,site,project}', 'policy_owner', 'seed', 'seed'),
  ('mdemg.classify.timeout_ms', 'integer', 'ms', 'Per-call budget for /jiminy/classify; a timeout counts as unreachable for the tier', '10000', '{"min":1000,"max":60000}', '{}', 'platform_admin', 'seed', 'seed')
ON CONFLICT (key) DO NOTHING;

-- A site-scoped override so the resolver has something to resolve in dev:
-- the Ontario site expresses its VP Finance threshold in CAD.
INSERT INTO policy_values (value_id, variable_key, scope_kind, scope_id, value, set_by, reason) VALUES
  ('01900000-0000-7000-8000-00000000c001', 'car.threshold.vp_finance', 'site', '01900000-0000-7000-8000-00000000a002', '6750000', 'dana.policy', 'CAD equivalent of the global USD threshold at the seed rate (dev only)')
ON CONFLICT (value_id) DO NOTHING;
