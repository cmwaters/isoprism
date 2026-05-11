-- Rename test flags to describe independent properties:
-- is_test marks code parsed from tests; is_entrypoint marks runnable/review entrypoints.

do $$
begin
  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_test_code'
  ) and not exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_test'
  ) then
    alter table code_nodes rename column is_test_code to is_test;
  end if;

  if not exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_test'
  ) then
    alter table code_nodes add column is_test boolean not null default false;
  end if;

  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_test_entrypoint'
  ) and not exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_entrypoint'
  ) then
    alter table code_nodes rename column is_test_entrypoint to is_entrypoint;
  end if;

  if not exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'code_nodes' and column_name = 'is_entrypoint'
  ) then
    alter table code_nodes add column is_entrypoint boolean not null default false;
  end if;
end $$;

drop index if exists code_nodes_repo_commit_test_idx;

create index if not exists code_nodes_repo_commit_test_idx
  on code_nodes (repo_id, commit_sha, is_test);
