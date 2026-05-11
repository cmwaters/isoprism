-- Preserve source comments that directly document parsed components.

alter table code_nodes
  add column if not exists doc_comment text;
