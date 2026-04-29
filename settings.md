# Isoprism Settings

> MVP specification | Updated: 2026-04-28

## 1. Purpose

Settings let a signed-in user manage the GitHub accounts, organizations, and repositories that Isoprism can work with.

The settings model should mirror GitHub's account model:

- A user has their own settings page at `/{user}/settings`
- An organization has its own settings page at `/{org}/settings`
- Both user and organization settings can manage GitHub App installation state, repository access, and future account-level settings

The first version should be small, reliable, and easy to extend. It should make GitHub access state clear without sending users directly to GitHub before they have landed in the relevant Isoprism settings context.

## 2. Global Account Button

The app shell should always show an account button in the top-right corner.

The button should:

- Be pill-shaped
- Show the signed-in user's GitHub avatar
- Show the signed-in user's GitHub name or username
- Navigate to the signed-in user's settings page, for example `/cmwaters/settings`
- Remain available from every authenticated view

If user data is loading, the button should render a compact loading state that preserves the same approximate dimensions.

## 3. Account Settings Routes

Settings are scoped to an account slug:

```text
/{account}/settings
```

The account may be either:

- A GitHub user account
- A GitHub organization account

Examples:

```text
/cmwaters/settings
/acme-inc/settings
```

The settings route should resolve the account slug and determine:

- Account type: user or organization
- Account avatar
- Account display name
- Whether the current signed-in user can manage the account in Isoprism
- GitHub App installation state for that account
- Repositories available to Isoprism for that account

## 4. Settings Layout

The settings page should use a side panel for settings categories.

Initial categories:

- Overview
- GitHub
- Repositories

Future categories may include:

- Members
- Access control
- Notifications
- Billing
- API keys
- Audit log

The side panel should remain stable as settings categories are added. The main content area should switch between categories without changing the account context.

## 5. User Settings

`/{user}/settings` is the signed-in user's home base for account and organization discovery.

It should show:

- The user's GitHub account connection
- The GitHub App installation state for the user's personal account
- Personal repositories available to Isoprism
- Organizations the user belongs to, where GitHub exposes that membership to the user's auth token

Organization rows should behave as internal navigation. Clicking an organization should go to:

```text
/{org}/settings
```

The user settings page should not send users directly to GitHub when they click an organization. GitHub install and permission management actions belong on the destination organization settings page.

Organization rows should show status clearly:

- Member
- GitHub App installed or not installed
- Number of repositories available, when known
- Enabled or disabled in Isoprism, if Isoprism keeps a separate product-level enabled state
- Whether the signed-in user can manage the organization in Isoprism

Example states:

```text
Acme Inc
Member · GitHub App installed · 12 repos available
Open settings

Beta Labs
Member · GitHub App not installed
Open settings

Personal Projects
Member · Installed · Disabled in Isoprism
Open settings
```

## 6. Organization Settings

`/{org}/settings` is where organization-specific GitHub and repository management happens.

It should show:

- Organization identity: avatar, name, and GitHub login
- GitHub App installation state for the organization
- Whether the signed-in user can manage the organization
- Repositories available to Isoprism through the organization's GitHub App installation
- Isoprism repository enablement state, if separate from GitHub App access

If the GitHub App is not installed for the organization, the organization settings page should explain that state and provide the GitHub install action.

If the GitHub App is installed for selected repositories, the page should show the accessible repositories and provide a manage action for updating GitHub App repository access.

If the GitHub App is installed for all repositories, the page should show all accessible repositories and provide a manage action for updating GitHub App access.

If the signed-in user cannot manage the organization, the page should be read-only and explain that an organization owner must install or update the GitHub App.

## 7. GitHub Access Model

The UI must distinguish between GitHub permission state and Isoprism product state.

GitHub controls:

- Whether the Isoprism GitHub App is installed on a user or organization account
- Whether the installation grants access to all repositories or selected repositories
- Which organizations the signed-in user belongs to and which memberships are visible to the authenticated token

Isoprism controls:

- Which accessible accounts are shown or enabled inside Isoprism
- Which accessible repositories are enabled, indexed, or hidden inside Isoprism
- Product-specific settings such as future members, access control, notifications, or billing

Seeing that a user belongs to an organization does not mean Isoprism can access that organization's repositories. Repository access requires the GitHub App to be installed on that organization and granted access to the relevant repositories.

## 8. GitHub Settings Category

The GitHub category should show installation and permission state for the current account.

For a user account, it should show:

- Connected GitHub user
- Personal GitHub App installation state
- Action to install the GitHub App for the personal account, when absent
- Action to manage the GitHub App installation, when present

For an organization account, it should show:

- Organization GitHub App installation state
- Whether the current signed-in user appears able to manage the installation
- Action to install the GitHub App for the organization, when absent
- Action to manage the organization installation, when present
- Read-only guidance when the user cannot manage the organization

GitHub actions may link out to GitHub, but only from the relevant account settings page.

## 9. Repositories Category

The repositories category should manage repositories for the current account context.

Each repository row should show:

- Repository owner
- Repository name
- Visibility: public or private
- GitHub access state
- Isoprism state: enabled, disabled, indexing, indexed, failed, or unavailable
- Available action

Initial actions:

- Add repository to Isoprism
- Remove repository from Isoprism
- Reindex repository, if already enabled
- Manage GitHub App access, if the repo is not available because GitHub has not granted access

Removing a repository from Isoprism should not uninstall the GitHub App or change GitHub-side repository permissions. It should only remove or disable the repository inside Isoprism.

## 10. MVP Scope

The first implementation should include:

- Global top-right account pill
- `/{user}/settings`
- `/{org}/settings`
- Shared settings layout with side panel navigation
- User settings overview with organizations list
- Organization settings overview
- GitHub installation state
- Repository list and basic add/remove repository actions

Out of scope for the first implementation:

- Full organization member management
- Role-based access control
- Billing
- Notification preferences
- Audit logs
- Direct GitHub organization membership changes

## 11. UX Principles

Settings should be operational and clear.

- Keep the layout quiet and scannable
- Use status chips for GitHub and Isoprism states
- Avoid ambiguous labels such as "Add org" or "Remove org" unless the product state is explicit
- Prefer "Open settings" for organization navigation
- Prefer "Install GitHub App" and "Manage GitHub App" for GitHub-side permission actions
- Prefer "Enable in Isoprism" and "Disable in Isoprism" for product-side account or repository state

The core rule: user settings are an overview and launcher; organization settings are where organization-specific work happens.
