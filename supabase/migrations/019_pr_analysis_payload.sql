alter table pr_analyses
  add column if not exists analysis_payload jsonb,
  add column if not exists prompt_version text;
