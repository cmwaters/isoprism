-- ============================================================
-- Isoprism — Graph Schema
-- ============================================================

-- ── 1. GitHub App installations ──────────────────────────────

create table github_installations (
  id                  uuid primary key default gen_random_uuid(),
  installation_id     bigint not null unique,
  account_login       text not null,
  account_type        text not null,
  account_avatar_url  text,
  created_at          timestamptz not null default now()
);

-- ── 2. Repositories (user-owned) ────────────────────────────

create table repositories (
  id               uuid primary key default gen_random_uuid(),
  user_id          uuid not null references auth.users(id) on delete cascade,
  installation_id  uuid not null references github_installations(id) on delete cascade,
  github_repo_id   bigint not null,
  full_name        text not null,
  default_branch   text not null default 'main',
  main_commit_sha  text,
  index_status     text not null default 'pending'
                   check (index_status in ('pending','running','ready','failed')),
  is_active        boolean not null default true,
  created_at       timestamptz not null default now(),
  unique (user_id, github_repo_id)
);

-- ── 3. Pull requests (per-repo) ──────────────────────────────

create table pull_requests (
  id               uuid primary key default gen_random_uuid(),
  repo_id          uuid not null references repositories(id) on delete cascade,
  github_pr_id     bigint not null,
  number           int not null,
  title            text not null,
  body             text,
  author_login     text not null,
  author_avatar_url text,
  base_branch      text not null,
  head_branch      text not null,
  base_commit_sha  text,
  head_commit_sha  text,
  state            text not null check (state in ('open','closed','merged')),
  draft            boolean not null default false,
  html_url         text not null,
  opened_at        timestamptz not null,
  merged_at        timestamptz,
  last_activity_at timestamptz,
  graph_status     text not null default 'pending'
                   check (graph_status in ('pending','running','ready','skipped','failed')),
  created_at       timestamptz not null default now(),
  unique (repo_id, github_pr_id)
);

-- ── 4. Code nodes (function-level snapshot, keyed by commit) ─

create table code_nodes (
  id          uuid primary key default gen_random_uuid(),
  repo_id     uuid not null references repositories(id) on delete cascade,
  commit_sha  text not null,
  full_name   text not null,
  file_path   text not null,
  line_start  int not null,
  line_end    int not null,
  inputs      jsonb not null default '[]'::jsonb,
  outputs     jsonb not null default '[]'::jsonb,
  language    text not null,
  kind        text not null,
  body_hash   text not null,
  summary     text,
  created_at  timestamptz not null default now(),
  unique (repo_id, commit_sha, full_name, file_path)
);

-- ── 5. Code edges (call graph at a given commit) ─────────────

create table code_edges (
  id          uuid primary key default gen_random_uuid(),
  repo_id     uuid not null references repositories(id) on delete cascade,
  commit_sha  text not null,
  caller_id   uuid not null references code_nodes(id) on delete cascade,
  callee_id   uuid not null references code_nodes(id) on delete cascade,
  created_at  timestamptz not null default now(),
  unique (repo_id, commit_sha, caller_id, callee_id)
);

-- ── 6. PR node changes (delta overlay) ───────────────────────

create table pr_node_changes (
  id              uuid primary key default gen_random_uuid(),
  pull_request_id uuid not null references pull_requests(id) on delete cascade,
  node_id         uuid not null references code_nodes(id) on delete cascade,
  change_type     text not null check (change_type in ('added','modified','deleted')),
  change_summary  text,
  diff_hunk       text,
  created_at      timestamptz not null default now(),
  unique (pull_request_id, node_id)
);

-- ── 7. PR analyses (queue display + urgency scoring) ─────────

create table pr_analyses (
  id              uuid primary key default gen_random_uuid(),
  pull_request_id uuid not null references pull_requests(id) on delete cascade unique,
  summary         text,
  nodes_changed   int not null default 0,
  risk_score      int check (risk_score between 1 and 10),
  risk_label      text check (risk_label in ('low','medium','high')),
  ai_model        text,
  generated_at    timestamptz,
  created_at      timestamptz not null default now()
);

-- ── 8. Indexes ───────────────────────────────────────────────

create index on repositories (user_id);
create index on repositories (installation_id);
create index on pull_requests (repo_id, state);
create index on pull_requests (repo_id, graph_status);
create index on code_nodes (repo_id, commit_sha);
create index on code_edges (repo_id, commit_sha);
create index on code_edges (caller_id);
create index on code_edges (callee_id);
create index on pr_node_changes (pull_request_id);
create index on pr_analyses (pull_request_id);

-- ── 9. Row-level security ───────────────────────────────────

alter table github_installations enable row level security;
alter table repositories enable row level security;
alter table pull_requests enable row level security;
alter table code_nodes enable row level security;
alter table code_edges enable row level security;
alter table pr_node_changes enable row level security;
alter table pr_analyses enable row level security;

-- Repositories: user can only see their own
create policy "users see own repositories"
  on repositories for select using (user_id = auth.uid());

-- Pull requests: user can see PRs in their repos
create policy "users see own repo prs"
  on pull_requests for select using (
    exists (select 1 from repositories r where r.id = repo_id and r.user_id = auth.uid())
  );

-- Code nodes: user can see nodes in their repos
create policy "users see own repo code_nodes"
  on code_nodes for select using (
    exists (select 1 from repositories r where r.id = repo_id and r.user_id = auth.uid())
  );

-- Code edges: user can see edges in their repos
create policy "users see own repo code_edges"
  on code_edges for select using (
    exists (select 1 from repositories r where r.id = repo_id and r.user_id = auth.uid())
  );

-- PR node changes: user can see changes in their PRs
create policy "users see own pr_node_changes"
  on pr_node_changes for select using (
    exists (
      select 1 from pull_requests pr
      join repositories r on r.id = pr.repo_id
      where pr.id = pull_request_id and r.user_id = auth.uid()
    )
  );

-- PR analyses: user can see analyses in their PRs
create policy "users see own pr_analyses"
  on pr_analyses for select using (
    exists (
      select 1 from pull_requests pr
      join repositories r on r.id = pr.repo_id
      where pr.id = pull_request_id and r.user_id = auth.uid()
    )
  );
