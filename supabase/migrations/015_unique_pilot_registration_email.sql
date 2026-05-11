create unique index if not exists pilot_forms_registration_email_unique_idx
on pilot_forms (lower(email))
where form_type = 'registration' and email is not null;
