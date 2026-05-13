alter table code_edges
  drop constraint if exists code_edges_edge_kind_check;

alter table code_edges
  add constraint code_edges_edge_kind_check
  check (edge_kind in ('calls', 'owns_method', 'uses_type'));
