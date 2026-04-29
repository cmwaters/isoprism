-- Invite-only beta administration.

create table if not exists beta_invites (
  id               uuid primary key default gen_random_uuid(),
  beta_id          text not null unique,
  name             text not null,
  token            text not null unique,
  email            text,
  status           text not null default 'new',
  invited_at       timestamptz not null default now(),
  expires_at       timestamptz,
  accepted_at      timestamptz,
  completed_at     timestamptz,
  user_id          uuid references auth.users(id) on delete set null,
  selected_repo_id uuid references repositories(id) on delete set null,
  trial_starts_at  timestamptz,
  trial_ends_at    timestamptz,
  created_at       timestamptz not null default now()
);

create table if not exists beta_feedback (
  id                uuid primary key default gen_random_uuid(),
  invite_id         uuid references beta_invites(id) on delete set null,
  user_id           uuid references auth.users(id) on delete set null,
  repo_id           uuid references repositories(id) on delete set null,
  pull_request_id   uuid references pull_requests(id) on delete set null,
  node_id           uuid references code_nodes(id) on delete set null,
  type              text not null,
  title             text not null,
  details           text not null,
  browser_path      text,
  github_issue_url  text,
  github_issue_number int,
  created_at        timestamptz not null default now()
);

create table if not exists beta_questionnaires (
  id                   uuid primary key default gen_random_uuid(),
  invite_id            uuid not null references beta_invites(id) on delete cascade,
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

create index if not exists beta_invites_status_idx on beta_invites (status);
create index if not exists beta_invites_user_id_idx on beta_invites (user_id);
create index if not exists beta_invites_selected_repo_id_idx on beta_invites (selected_repo_id);
create index if not exists beta_feedback_invite_id_idx on beta_feedback (invite_id);
create index if not exists beta_questionnaires_invite_id_idx on beta_questionnaires (invite_id);

alter table beta_invites enable row level security;
alter table beta_feedback enable row level security;
alter table beta_questionnaires enable row level security;
