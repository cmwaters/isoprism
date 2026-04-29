create table indexing_jobs (
  id             uuid primary key default gen_random_uuid(),
  repo_id        uuid not null references repositories(id) on delete cascade,
  commit_sha     text not null,
  status         text not null default 'pending'
                 check (status in ('pending','running','ready','failed')),
  phase          text not null default 'pending',
  message        text,
  files_total    int not null default 0,
  files_done     int not null default 0,
  nodes_total    int not null default 0,
  nodes_done     int not null default 0,
  edges_total    int not null default 0,
  edges_done     int not null default 0,
  started_at     timestamptz,
  updated_at     timestamptz not null default now(),
  finished_at    timestamptz,
  error          text,
  created_at     timestamptz not null default now(),
  unique (repo_id, commit_sha)
);

create index on indexing_jobs (repo_id, created_at desc);
create index on indexing_jobs (repo_id, commit_sha);

alter table indexing_jobs enable row level security;

create policy "users see own repo indexing_jobs"
  on indexing_jobs for select using (
    exists (select 1 from repositories r where r.id = repo_id and r.user_id = auth.uid())
  );
