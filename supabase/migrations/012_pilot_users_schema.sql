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
