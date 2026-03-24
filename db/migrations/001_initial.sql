-- ============================================================
-- Aperture — Initial Schema (v2: orgs + teams)
-- ============================================================

create extension if not exists "pgcrypto";

-- ============================================================
-- Users
-- ============================================================
create table users (
  id               uuid primary key, -- matches auth.users.id
  email            text not null,
  display_name     text,
  avatar_url       text,
  github_user_id   bigint unique,
  github_username  text,
  created_at       timestamptz not null default now()
);

create or replace function handle_new_auth_user()
returns trigger language plpgsql security definer as $$
begin
  insert into public.users (id, email, display_name, avatar_url, github_user_id, github_username)
  values (
    new.id,
    new.email,
    coalesce(new.raw_user_meta_data->>'full_name', new.raw_user_meta_data->>'name'),
    new.raw_user_meta_data->>'avatar_url',
    (new.raw_user_meta_data->>'provider_id')::bigint,
    new.raw_user_meta_data->>'user_name'
  )
  on conflict (id) do update set
    display_name = excluded.display_name,
    avatar_url = excluded.avatar_url,
    github_user_id = excluded.github_user_id,
    github_username = excluded.github_username;
  return new;
end;
$$;

create trigger on_auth_user_created
  after insert or update on auth.users
  for each row execute procedure handle_new_auth_user();

-- ============================================================
-- Organizations
-- ============================================================
create table organizations (
  id                    uuid primary key default gen_random_uuid(),
  name                  text not null,
  slug                  text not null unique,
  github_account_login  text not null unique,
  github_account_type   text not null check (github_account_type in ('Organization', 'User')),
  github_account_id     bigint,
  avatar_url            text,
  created_at            timestamptz not null default now()
);

-- ============================================================
-- Org Members
-- ============================================================
create table org_members (
  id          uuid primary key default gen_random_uuid(),
  org_id      uuid not null references organizations(id) on delete cascade,
  user_id     uuid not null references users(id) on delete cascade,
  role        text not null default 'member' check (role in ('org_admin', 'member')),
  created_at  timestamptz not null default now(),
  unique (org_id, user_id)
);

-- ============================================================
-- Teams (flat, within orgs)
-- ============================================================
create table teams (
  id              uuid primary key default gen_random_uuid(),
  org_id          uuid not null references organizations(id) on delete cascade,
  name            text not null,
  slug            text not null,
  github_team_id  bigint,
  created_at      timestamptz not null default now(),
  unique (org_id, slug)
);

-- ============================================================
-- Team Members
-- ============================================================
create table team_members (
  id          uuid primary key default gen_random_uuid(),
  team_id     uuid not null references teams(id) on delete cascade,
  user_id     uuid not null references users(id) on delete cascade,
  role        text not null default 'member' check (role in ('team_admin', 'member')),
  created_at  timestamptz not null default now(),
  unique (team_id, user_id)
);

-- ============================================================
-- GitHub App Installations
-- ============================================================
create table github_installations (
  id                  uuid primary key default gen_random_uuid(),
  org_id              uuid not null references organizations(id) on delete cascade,
  installation_id     bigint not null unique,
  account_login       text not null,
  account_type        text not null check (account_type in ('Organization', 'User')),
  account_avatar_url  text,
  created_at          timestamptz not null default now()
);

-- ============================================================
-- Repositories
-- ============================================================
create table repositories (
  id               uuid primary key default gen_random_uuid(),
  org_id           uuid not null references organizations(id) on delete cascade,
  installation_id  uuid not null references github_installations(id) on delete cascade,
  github_repo_id   bigint not null,
  full_name        text not null,
  default_branch   text not null default 'main',
  is_active        boolean not null default true,
  created_at       timestamptz not null default now(),
  unique (org_id, github_repo_id)
);

-- ============================================================
-- Team Repos (which repos a team watches)
-- ============================================================
create table team_repos (
  team_id  uuid not null references teams(id) on delete cascade,
  repo_id  uuid not null references repositories(id) on delete cascade,
  primary key (team_id, repo_id)
);

-- ============================================================
-- Pull Requests
-- ============================================================
create table pull_requests (
  id                    uuid primary key default gen_random_uuid(),
  org_id                uuid not null references organizations(id) on delete cascade,
  repo_id               uuid not null references repositories(id) on delete cascade,
  github_pr_id          bigint not null,
  number                int not null,
  title                 text not null,
  body                  text,
  author_github_login   text not null,
  author_avatar_url     text,
  base_branch           text not null,
  head_branch           text not null,
  state                 text not null check (state in ('open', 'closed', 'merged')),
  draft                 boolean not null default false,
  additions             int not null default 0,
  deletions             int not null default 0,
  changed_files         int not null default 0,
  html_url              text not null,
  opened_at             timestamptz not null,
  closed_at             timestamptz,
  merged_at             timestamptz,
  last_activity_at      timestamptz,
  last_synced_at        timestamptz,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now(),
  unique (repo_id, github_pr_id)
);

-- ============================================================
-- PR Analyses (AI-generated)
-- ============================================================
create table pr_analyses (
  id                uuid primary key default gen_random_uuid(),
  pull_request_id   uuid not null references pull_requests(id) on delete cascade,
  commit_sha        text not null,
  summary           text,
  why               text,
  impacted_areas    text[] not null default '{}',
  key_files         text[] not null default '{}',
  size_label        text check (size_label in ('small', 'medium', 'large')),
  risk_score        int check (risk_score between 1 and 10),
  risk_label        text check (risk_label in ('low', 'medium', 'high')),
  risk_reasons      text[] not null default '{}',
  semantic_groups   jsonb,
  ai_provider       text,
  ai_model          text,
  generated_at      timestamptz,
  created_at        timestamptz not null default now()
);

-- ============================================================
-- PR Reviews (synced from GitHub)
-- ============================================================
create table pr_reviews (
  id                   uuid primary key default gen_random_uuid(),
  pull_request_id      uuid not null references pull_requests(id) on delete cascade,
  github_review_id     bigint not null unique,
  reviewer_login       text not null,
  reviewer_avatar_url  text,
  state                text not null check (state in ('approved', 'changes_requested', 'commented', 'dismissed')),
  submitted_at         timestamptz not null
);

-- ============================================================
-- Org Preferences
-- ============================================================
create table org_preferences (
  id                  uuid primary key default gen_random_uuid(),
  org_id              uuid not null references organizations(id) on delete cascade unique,
  pr_size_small_max   int not null default 100,
  pr_size_medium_max  int not null default 400,
  stale_after_hours   int not null default 48,
  risk_sensitivity    text not null default 'medium' check (risk_sensitivity in ('low', 'medium', 'high')),
  ai_provider         text not null default 'anthropic' check (ai_provider in ('anthropic', 'openai')),
  created_at          timestamptz not null default now(),
  updated_at          timestamptz not null default now()
);

-- ============================================================
-- Org Join Requests
-- ============================================================
create table org_join_requests (
  id           uuid primary key default gen_random_uuid(),
  org_id       uuid not null references organizations(id) on delete cascade,
  user_id      uuid not null references users(id) on delete cascade,
  status       text not null default 'pending' check (status in ('pending', 'approved', 'rejected')),
  created_at   timestamptz not null default now(),
  resolved_at  timestamptz,
  resolved_by  uuid references users(id),
  unique (org_id, user_id)
);

-- ============================================================
-- Indexes
-- ============================================================
create index on pull_requests (org_id, state);
create index on pull_requests (repo_id, state);
create index on pull_requests (last_activity_at desc);
create index on pr_analyses (pull_request_id);
create index on pr_reviews (pull_request_id);
create index on org_members (user_id);
create index on team_members (user_id);
create index on repositories (org_id);

-- ============================================================
-- Row Level Security
-- ============================================================
alter table organizations enable row level security;
alter table users enable row level security;
alter table org_members enable row level security;
alter table teams enable row level security;
alter table team_members enable row level security;
alter table github_installations enable row level security;
alter table repositories enable row level security;
alter table pull_requests enable row level security;
alter table pr_analyses enable row level security;
alter table pr_reviews enable row level security;
alter table org_preferences enable row level security;
alter table org_join_requests enable row level security;
alter table team_repos enable row level security;

-- Helper: is the current auth user a member of a given org?
create or replace function is_org_member(p_org_id uuid)
returns boolean language sql security definer as $$
  select exists (
    select 1 from org_members
    where org_members.org_id = p_org_id
      and org_members.user_id = auth.uid()
  );
$$;

-- Helper: is the current auth user an admin of a given org?
create or replace function is_org_admin(p_org_id uuid)
returns boolean language sql security definer as $$
  select exists (
    select 1 from org_members
    where org_members.org_id = p_org_id
      and org_members.user_id = auth.uid()
      and org_members.role = 'org_admin'
  );
$$;

-- Organizations
create policy "org members can read their org"
  on organizations for select using (is_org_member(id));

-- Org members
create policy "org members can read memberships"
  on org_members for select using (is_org_member(org_id));
create policy "org admins can insert memberships"
  on org_members for insert with check (
    auth.uid() = user_id or is_org_admin(org_id)
  );

-- Teams
create policy "org members can read teams"
  on teams for select using (is_org_member(org_id));
create policy "org admins can insert teams"
  on teams for insert with check (is_org_admin(org_id));

-- Team members
create policy "org members can read team members"
  on team_members for select using (
    exists (select 1 from teams t where t.id = team_id and is_org_member(t.org_id))
  );

-- Users
create policy "users can read own profile"
  on users for select using (id = auth.uid());
create policy "users can update own profile"
  on users for update using (id = auth.uid());

-- GitHub installations
create policy "org members can read github_installations"
  on github_installations for select using (is_org_member(org_id));

-- Repositories
create policy "org members can read repositories"
  on repositories for select using (is_org_member(org_id));
create policy "org admins can update repositories"
  on repositories for update using (is_org_admin(org_id));

-- Team repos
create policy "org members can read team_repos"
  on team_repos for select using (
    exists (select 1 from teams t where t.id = team_id and is_org_member(t.org_id))
  );

-- Pull requests
create policy "org members can read pull_requests"
  on pull_requests for select using (is_org_member(org_id));

-- PR analyses
create policy "org members can read pr_analyses"
  on pr_analyses for select using (
    exists (
      select 1 from pull_requests pr
      where pr.id = pull_request_id and is_org_member(pr.org_id)
    )
  );

-- PR reviews
create policy "org members can read pr_reviews"
  on pr_reviews for select using (
    exists (
      select 1 from pull_requests pr
      where pr.id = pull_request_id and is_org_member(pr.org_id)
    )
  );

-- Org preferences
create policy "org members can read org_preferences"
  on org_preferences for select using (is_org_member(org_id));
create policy "org admins can upsert org_preferences"
  on org_preferences for insert with check (is_org_admin(org_id));
create policy "org admins can update org_preferences"
  on org_preferences for update using (is_org_admin(org_id));

-- Join requests
create policy "users can create join requests"
  on org_join_requests for insert with check (auth.uid() = user_id);
create policy "users can read own join requests"
  on org_join_requests for select using (
    auth.uid() = user_id or is_org_admin(org_id)
  );
create policy "org admins can update join requests"
  on org_join_requests for update using (is_org_admin(org_id));
