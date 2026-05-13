alter table code_edges
  rename column caller_id to source_id;

alter table code_edges
  rename column callee_id to destination_id;

alter table code_edges
  add column if not exists edge_kind text not null default 'calls';

alter table code_edges
  drop constraint if exists code_edges_repo_id_commit_sha_caller_id_callee_id_key;

alter table code_edges
  drop constraint if exists code_edges_edge_kind_check;

alter table code_edges
  add constraint code_edges_edge_kind_check
  check (edge_kind in ('calls', 'owns_method'));

alter table code_edges
  add constraint code_edges_repo_commit_source_destination_kind_key
  unique (repo_id, commit_sha, source_id, destination_id, edge_kind);

drop index if exists code_edges_caller_id_idx;
drop index if exists code_edges_callee_id_idx;

create index if not exists code_edges_source_id_idx
  on code_edges (source_id);

create index if not exists code_edges_destination_id_idx
  on code_edges (destination_id);

create index if not exists code_edges_repo_commit_kind_idx
  on code_edges (repo_id, commit_sha, edge_kind);

insert into code_edges (repo_id, commit_sha, source_id, destination_id, edge_kind)
select owner.repo_id, owner.commit_sha, owner.id, method.id, 'owns_method'
from code_nodes method
join code_nodes owner
  on owner.repo_id = method.repo_id
 and owner.commit_sha = method.commit_sha
 and owner.full_name = regexp_replace(method.full_name, '\.[^.]+$', '')
where method.kind = 'method'
  and owner.kind in ('struct', 'type', 'interface')
  and owner.id <> method.id
on conflict do nothing;
