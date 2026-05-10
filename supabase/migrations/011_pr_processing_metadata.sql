alter table pull_requests
  add column if not exists processor_commit_sha text,
  add column if not exists processed_at timestamptz,
  add column if not exists processing_error text,
  add column if not exists processing_stats jsonb not null default '{}'::jsonb;
