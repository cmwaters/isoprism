drop table if exists beta_feedback;

do $$
begin
  if to_regclass('public.beta_invites') is not null
     and to_regclass('public.pilot_users') is null then
    alter table beta_invites rename to pilot_users;
  end if;
end $$;

alter table if exists pilot_users
  drop column if exists beta_id;

alter table if exists pilot_users
  alter column token drop not null,
  alter column invited_at drop not null,
  alter column invited_at drop default,
  add column if not exists invited_at timestamptz,
  add column if not exists review_token text unique,
  add column if not exists review_sent_at timestamptz,
  add column if not exists pilot_languages text,
  add column if not exists public_repo_url text,
  add column if not exists issue_count int not null default 0,
  add column if not exists feature_count int not null default 0;

create table if not exists pilot_forms (
  id            uuid primary key default gen_random_uuid(),
  pilot_user_id uuid references pilot_users(id) on delete set null,
  form_type     text not null,
  name          text,
  email         text,
  answers       jsonb not null default '{}'::jsonb,
  submitted_at  timestamptz not null default now(),
  created_at    timestamptz not null default now()
);

create index if not exists pilot_forms_type_idx on pilot_forms (form_type);
create index if not exists pilot_forms_user_id_idx on pilot_forms (pilot_user_id);

do $$
begin
  if to_regclass('public.beta_questionnaires') is not null
     and to_regclass('public.pilot_questionaire') is null then
    alter table beta_questionnaires rename to pilot_questionaire;
  end if;
end $$;

do $$
begin
  if to_regclass('public.beta_invites_status_idx') is not null
     and to_regclass('public.pilot_users_status_idx') is null then
    alter index beta_invites_status_idx rename to pilot_users_status_idx;
  end if;
  if to_regclass('public.beta_invites_user_id_idx') is not null
     and to_regclass('public.pilot_users_user_id_idx') is null then
    alter index beta_invites_user_id_idx rename to pilot_users_user_id_idx;
  end if;
  if to_regclass('public.beta_invites_selected_repo_id_idx') is not null
     and to_regclass('public.pilot_users_selected_repo_id_idx') is null then
    alter index beta_invites_selected_repo_id_idx rename to pilot_users_selected_repo_id_idx;
  end if;
  if to_regclass('public.beta_invites_token_idx') is not null
     and to_regclass('public.pilot_users_token_idx') is null then
    alter index beta_invites_token_idx rename to pilot_users_token_idx;
  end if;
  if to_regclass('public.beta_questionnaires_invite_id_idx') is not null
     and to_regclass('public.pilot_questionaire_invite_id_idx') is null then
    alter index beta_questionnaires_invite_id_idx rename to pilot_questionaire_invite_id_idx;
  end if;
end $$;
