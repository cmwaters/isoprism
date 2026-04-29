-- Store test entrypoints that exercise production graph nodes.
-- Test code is intentionally excluded from code_nodes/code_edges.

create table code_test_references (
  id               uuid primary key default gen_random_uuid(),
  repo_id          uuid not null references repositories(id) on delete cascade,
  commit_sha       text not null,
  test_name        text not null,
  test_full_name   text not null,
  test_file_path   text not null,
  test_line_start  int not null,
  test_line_end    int not null,
  target_node_id   uuid not null references code_nodes(id) on delete cascade,
  created_at       timestamptz not null default now(),
  unique (repo_id, commit_sha, test_full_name, test_file_path, target_node_id)
);

create index on code_test_references (repo_id, commit_sha);
create index on code_test_references (target_node_id);

alter table code_test_references enable row level security;

create policy "users see own repo code_test_references"
  on code_test_references for select using (
    exists (select 1 from repositories r where r.id = repo_id and r.user_id = auth.uid())
  );
