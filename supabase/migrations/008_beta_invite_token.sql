alter table beta_invites
  add column if not exists token text;

update beta_invites
set token = token_plain
where token is null
  and token_plain is not null;

create unique index if not exists beta_invites_token_idx on beta_invites (token);

alter table beta_invites
  drop column if exists token_plain,
  drop column if exists token_hash;
