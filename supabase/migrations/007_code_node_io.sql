alter table code_nodes
  add column if not exists inputs jsonb not null default '[]'::jsonb,
  add column if not exists outputs jsonb not null default '[]'::jsonb;

alter table code_nodes
  drop column if exists signature,
  drop column if exists name;
