-- Align the pilot review rollup table with the current end-of-pilot form.

alter table if exists pilot_questionaire
  add column if not exists keep_using_reason text,
  add column if not exists most_important_features text,
  add column if not exists not_keep_using_reason text,
  add column if not exists switch_requirements text,
  add column if not exists open_to_follow_up text,
  drop column if exists faster_rating,
  drop column if exists risk_clarity_rating,
  drop column if exists confusing_or_missing,
  drop column if exists bugs_hit,
  drop column if exists build_next;
