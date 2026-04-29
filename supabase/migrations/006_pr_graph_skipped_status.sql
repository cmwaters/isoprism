alter table pull_requests
  drop constraint if exists pull_requests_graph_status_check;

alter table pull_requests
  add constraint pull_requests_graph_status_check
  check (graph_status in ('pending','running','ready','skipped','failed'));
