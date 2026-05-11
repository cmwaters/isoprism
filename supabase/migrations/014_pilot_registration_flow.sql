alter table if exists pilot_users
  alter column token drop not null,
  alter column invited_at drop not null,
  alter column invited_at drop default,
  add column if not exists review_token text unique,
  add column if not exists review_sent_at timestamptz,
  add column if not exists pilot_languages text,
  add column if not exists public_repo_url text,
  add column if not exists issue_count int not null default 0,
  add column if not exists feature_count int not null default 0;

create table if not exists pilot_forms (
  id            uuid primary key default gen_random_uuid(),
  pilot_user_id uuid references pilot_users(id) on delete set null,
  form_type     text not null,
  name          text,
  email         text,
  answers       jsonb not null default '{}'::jsonb,
  submitted_at  timestamptz not null default now(),
  created_at    timestamptz not null default now()
);

create index if not exists pilot_forms_type_idx on pilot_forms (form_type);
create index if not exists pilot_forms_user_id_idx on pilot_forms (pilot_user_id);

alter table pilot_forms enable row level security;
