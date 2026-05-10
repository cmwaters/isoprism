-- PR diff parity: preserve semantic renames and old symbol metadata.

alter table pr_node_changes
  drop constraint if exists pr_node_changes_change_type_check;

alter table pr_node_changes
  add constraint pr_node_changes_change_type_check
  check (change_type in ('added','modified','deleted','renamed'));

alter table pr_node_changes
  add column if not exists old_full_name text,
  add column if not exists old_file_path text;
