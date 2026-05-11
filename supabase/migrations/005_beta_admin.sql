-- Pilot administration.

create table if not exists pilot_users (
  id               uuid primary key default gen_random_uuid(),
  name             text not null,
  token            text unique,
  email            text,
  status           text not null default 'new',
  invited_at       timestamptz,
  expires_at       timestamptz,
  accepted_at      timestamptz,
  completed_at     timestamptz,
  review_token     text unique,
  review_sent_at   timestamptz,
  pilot_languages  text,
  public_repo_url  text,
  issue_count      int not null default 0,
  feature_count    int not null default 0,
  user_id          uuid references auth.users(id) on delete set null,
  selected_repo_id uuid references repositories(id) on delete set null,
  trial_starts_at  timestamptz,
  trial_ends_at    timestamptz,
  created_at       timestamptz not null default now()
);

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

create table if not exists pilot_questionaire (
  id                   uuid primary key default gen_random_uuid(),
  invite_id            uuid not null references pilot_users(id) on delete cascade,
  user_id              uuid references auth.users(id) on delete set null,
  repo_id              uuid references repositories(id) on delete set null,
  faster_rating        int,
  risk_clarity_rating  int,
  confusing_or_missing text,
  bugs_hit             text,
  build_next           text,
  would_keep_using     text,
  submitted_at         timestamptz not null default now(),
  created_at           timestamptz not null default now(),
  unique (invite_id)
);

create index if not exists pilot_users_status_idx on pilot_users (status);
create index if not exists pilot_users_user_id_idx on pilot_users (user_id);
create index if not exists pilot_users_selected_repo_id_idx on pilot_users (selected_repo_id);
create index if not exists pilot_forms_type_idx on pilot_forms (form_type);
create index if not exists pilot_forms_user_id_idx on pilot_forms (pilot_user_id);
create index if not exists pilot_questionaire_invite_id_idx on pilot_questionaire (invite_id);

alter table pilot_users enable row level security;
alter table pilot_forms enable row level security;
alter table pilot_questionaire enable row level security;
