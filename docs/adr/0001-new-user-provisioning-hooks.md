# New-user provisioning via user-created hooks; defaults are a creation-time snapshot

New users need side effects beyond the bare account row: Shortcut Commands defaults
today, likely more (default groups, default settings) later. We seed these through a
lightweight `UserCreatedHook` registry invoked after user creation, instead of inlining
side effects into `UserHandler.Create` or building a general event bus. Hooks fire only
at account creation, so New-User Defaults are a creation-time snapshot: when the default
set changes later, existing users are never retroactively seeded — they add new ones
manually.

## Considered Options

- **Inline in `UserHandler.Create`** — rejected: every future feature would touch the
  handler and it silently couples account creation to unrelated provisioning concerns.
- **General event bus (pub-sub)** — rejected: no cross-context consumers exist yet; a
  bus is premature and harder to follow than an ordered hook list.
- **UserCreatedHook registry (chosen)** — one interface, a slice on the server, append
  when a feature needs provisioning; keeps `UserHandler` stable.

## Consequences

- Existing users never receive future defaults; provisioning is one-shot at creation.
- Hook failures are logged and never block account creation (provisioning is recoverable
  personal data, not part of the create transaction).
