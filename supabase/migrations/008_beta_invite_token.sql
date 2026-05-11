alter table pilot_users
  add column if not exists token text;

do $$
begin
  if exists (
    select 1
    from information_schema.columns
    where table_schema = 'public'
      and table_name = 'pilot_users'
      and column_name = 'token_plain'
  ) then
    update pilot_users
    set token = token_plain
    where token is null
      and token_plain is not null;
  end if;
end $$;

create unique index if not exists pilot_users_token_idx on pilot_users (token);

alter table pilot_users
  drop column if exists token_plain,
  drop column if exists token_hash;
