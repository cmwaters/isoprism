-- ============================================================
-- Aperture — Queue Signals (v2)
-- Adds data needed for the personalized PR ranking algorithm:
--   • head_sha / mergeable on pull_requests
--   • commit_sha on pr_reviews (detect stale approvals)
--   • pr_check_runs  — CI status per head commit
--   • pr_review_threads — unresolved conversation tracking
--   • pr_review_requests — explicit reviewer assignments
-- ============================================================

-- ---- pull_requests additions ----
alter table pull_requests
    add column if not exists head_sha  text,
    add column if not exists mergeable boolean;  -- null = unknown (computed async by GitHub)

-- ---- pr_reviews additions ----
alter table pr_reviews
    add column if not exists commit_sha text;  -- SHA when the review was submitted

-- ============================================================
-- pr_check_runs
-- One row per (pr, ci-app, head-sha). Updated on check_suite events.
-- ============================================================
create table if not exists pr_check_runs (
    id              uuid        primary key default gen_random_uuid(),
    pull_request_id uuid        not null references pull_requests(id) on delete cascade,
    head_sha        text        not null,
    app_slug        text        not null default '',
    status          text        not null, -- 'queued' | 'in_progress' | 'completed'
    conclusion      text,                 -- 'success'|'failure'|'neutral'|'cancelled'|'skipped'|'timed_out'|'action_required'|null
    updated_at      timestamptz not null default now(),
    unique (pull_request_id, app_slug, head_sha)
);

-- ============================================================
-- pr_review_threads
-- One row per review thread (root comment). Updated on
-- pull_request_review_comment + pull_request_review_thread events.
-- ============================================================
create table if not exists pr_review_threads (
    id                   uuid        primary key default gen_random_uuid(),
    pull_request_id      uuid        not null references pull_requests(id) on delete cascade,
    github_thread_id     bigint      not null,  -- root comment ID
    is_resolved          boolean     not null default false,
    last_commenter_login text,
    updated_at           timestamptz not null default now(),
    unique (pull_request_id, github_thread_id)
);

-- ============================================================
-- pr_review_requests
-- One row per requested reviewer. Cleared when a review is submitted
-- or the request is removed.
-- ============================================================
create table if not exists pr_review_requests (
    id              uuid        primary key default gen_random_uuid(),
    pull_request_id uuid        not null references pull_requests(id) on delete cascade,
    reviewer_login  text        not null,
    requested_at    timestamptz not null default now(),
    unique (pull_request_id, reviewer_login)
);

-- ============================================================
-- Indexes
-- ============================================================
create index if not exists pr_check_runs_pr_id    on pr_check_runs(pull_request_id);
create index if not exists pr_review_threads_pr_id on pr_review_threads(pull_request_id);
create index if not exists pr_review_requests_pr_id on pr_review_requests(pull_request_id);
create index if not exists pull_requests_head_sha  on pull_requests(head_sha) where head_sha is not null;

-- ============================================================
-- Row Level Security
-- ============================================================
alter table pr_check_runs     enable row level security;
alter table pr_review_threads enable row level security;
alter table pr_review_requests enable row level security;

create policy "org members can read pr_check_runs"
    on pr_check_runs for select
    using (
        exists (
            select 1 from pull_requests pr
            join org_members om on om.org_id = pr.org_id
            where pr.id = pr_check_runs.pull_request_id
              and om.user_id = auth.uid()
        )
    );

create policy "org members can read pr_review_threads"
    on pr_review_threads for select
    using (
        exists (
            select 1 from pull_requests pr
            join org_members om on om.org_id = pr.org_id
            where pr.id = pr_review_threads.pull_request_id
              and om.user_id = auth.uid()
        )
    );

create policy "org members can read pr_review_requests"
    on pr_review_requests for select
    using (
        exists (
            select 1 from pull_requests pr
            join org_members om on om.org_id = pr.org_id
            where pr.id = pr_review_requests.pull_request_id
              and om.user_id = auth.uid()
        )
    );
