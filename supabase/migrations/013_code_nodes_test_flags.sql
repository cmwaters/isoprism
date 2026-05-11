alter table code_nodes
  add column if not exists is_test_code boolean not null default false,
  add column if not exists is_test_entrypoint boolean not null default false;

create index if not exists code_nodes_repo_commit_test_idx
  on code_nodes (repo_id, commit_sha, is_test_code);

drop table if exists code_test_references;
