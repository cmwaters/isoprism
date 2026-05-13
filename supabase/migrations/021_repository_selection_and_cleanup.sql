-- Repository selection and delayed indexed-data cleanup.

alter table if exists users
  add column if not exists selected_repo_id uuid references repositories(id) on delete set null;

alter table if exists users
  add column if not exists account_class text not null default 'regular'
    check (account_class in ('pilot', 'regular'));

alter table if exists repositories
  add column if not exists github_access_status text not null default 'authorized'
    check (github_access_status in ('authorized', 'revoked'));

alter table if exists repositories
  add column if not exists authorized_at timestamptz not null default now();

alter table if exists repositories
  add column if not exists revoked_at timestamptz;

alter table if exists repositories
  add column if not exists indexed_at timestamptz;

alter table if exists repositories
  add column if not exists selected_at timestamptz;

alter table if exists repositories
  add column if not exists unused_at timestamptz;

alter table if exists repositories
  add column if not exists purge_after timestamptz;

update repositories
set github_access_status = case when is_active then 'authorized' else 'revoked' end,
    revoked_at = case when is_active then null else coalesce(revoked_at, now()) end,
    purge_after = case when is_active then purge_after else coalesce(purge_after, now() + interval '1 day') end;

update repositories
set indexed_at = coalesce(indexed_at, created_at)
where index_status = 'ready' and indexed_at is null;

update users u
set selected_repo_id = p.selected_repo_id,
    account_class = 'pilot'
from pilot_users p
where p.user_id = u.id
  and p.selected_repo_id is not null
  and u.selected_repo_id is null;

create index if not exists users_selected_repo_id_idx on users (selected_repo_id);
create index if not exists repositories_github_access_status_idx on repositories (github_access_status);
create index if not exists repositories_purge_after_idx on repositories (purge_after) where purge_after is not null;
create index if not exists repositories_user_selected_idx on repositories (user_id, selected_at desc) where selected_at is not null;
