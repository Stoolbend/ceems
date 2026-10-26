# Agent working agreement

These rules apply to every agent run in this repository. This file holds the
roles and the developer process. `../ceems-dev/agent-coordinator.md` holds
the coordinator protocol. A task packet, `../ceems-dev/packets/<id>.md`,
holds the facts of one task.

The process files (packets, the run ledger, evidence, the coordinator protocol,
the task template and the project brief) live in a private companion
repository, `ceems-dev`, checked out beside this one (on the owner's machine,
`C:\Code\ceems-dev`). An outside contributor can ignore them: this
file and the code are all a pull request needs.

## Roles

A session or subagent holds exactly one role. Roles do not blur: the
coordinator does not implement, the architect does not edit, the developer does
not decide scope.

| Role | Runs as | Model, effort | Does | Never |
|---|---|---|---|---|
| Owner | the person | — | Product, goal and authority decisions. Pushes. Rotates credentials | — |
| Coordinator | the session | Opus, medium | Writes packets, allocates worktrees, dispatches, verifies, integrates. Keeps `../ceems-dev/agent-runs.md` and `docs/decisions.md` current | Implements a packet. Edits an active worktree. Widens scope by chat |
| Architect | `packet-architect` | Fable, medium | Settles the design decisions a packet depends on. Reviews a returned diff for conformance to the packet's design. Drafts ADRs | Edits a file. Decides a product or goal question |
| Developer | `packet-developer`, or `packet-developer-big` when the packet says so | Sonnet, medium by default. Opus, medium via `packet-developer-big` when the packet requires judgement the design record does not settle | Executes one packet in its task worktree, commits locally, writes the handover | Coordinates, expands scope, integrates, pushes, creates or removes worktrees |
| Researcher | `packet-researcher` | Sonnet, medium. Opus, medium by `model: opus` on the dispatch for a bigger job, or when a Sonnet answer left a gap on the load-bearing point | Answers one question with `path:line` evidence | Changes a file |
| Checker | `packet-checker` | Haiku (no effort setting) | Runs named commands and reports raw results | Chooses checks, fixes, diagnoses |

Agent models and efforts are set in each `.claude/agents/*.md` frontmatter
(`model:` and `effort:`). This table and those files must agree; change both
together. Claude Code discovers project subagents only under `.claude/agents/`.

Escalation goes one way, and always through the coordinator:

- A developer, researcher or checker that meets a design or logic choice the
  packet does not settle stops and asks the coordinator. It does not guess.
- The coordinator sends design questions to the architect and product, goal or
  authority questions to the owner, then writes the answer into the packet
  before work resumes. An answer given only in chat is not a scope change.
- The architect returns product and goal questions to the coordinator.

Reading is not decision-making. Any role may read packets, diffs, evidence and
the ledger directly. Broad code reading is delegated to the researcher when it
would crowd out the role's own work.

## Process weight

This is a single-owner hobby project. Spend agent runs where a mistake is
expensive, and keep everything else light. The packet header records which
weight applies.

| Weight | Use for | Architect | Conformance review | Mutation check |
|---|---|---|---|---|
| `light` | Docs, config, a small fix, a change confined to a few files with an obvious design | Skip | Skip; the coordinator reads the diff | Not required |
| `standard` | A feature slice, a new module, anything touching a data model or an API shape | Only if a structural choice is open | When the packet has a design record or the diff touches more than about ten files | Only for the criteria the packet names |
| `critical` | Authentication, authorisation and access scoping, anything that deletes or overwrites user data, jobs that write unattended | Required | Required | Required for every rejection or permission rule the packet asserts |

The owner may raise or lower a packet's weight. The developer may not.

## Developer process

The packet supplies scope. This section supplies the repeatable process.

### Start

1. Read the task packet named by the user, in full, including anything appended
   after the original brief. A coordinator follow-up at the end supersedes the
   brief above it.
2. For every repository listed by the packet, read that repository's root
   `AGENTS.md` before changing it.
3. Confirm each repository root, worktree path, branch and `HEAD` matches the
   packet. Never write to an original checkout when the packet names a task
   worktree.
4. Read only the files and references the packet names, then inspect more code
   only when needed to trace the affected flow.
5. Check `git status --short` in every listed worktree before editing.
6. If the packet conflicts with this file, follow the packet and record the
   exception in the handover.

Do not spend the first turn rewriting the plan. Start the work unless a missing
decision would materially change the result; if one would, stop and ask.

### Work

- Stay inside the packet's scope and repository list.
- Prefer the smallest change that meets every acceptance criterion. Reuse
  existing code and platform features before adding abstractions or packages.
- Fix root causes at the narrowest shared layer. Do not patch only one caller
  when sibling callers have the same fault.
- Preserve user changes. Do not commit unrelated files.
- For multi-repository tasks, treat the listed worktree paths as one task
  workspace. Do not substitute the repositories' original checkout paths.
- Never commit or print credentials, tokens, private URLs or unsanitised
  captures. The rule governs what you look at, not only what you commit.
  **The operator's machine is not a safe place to read from.** Browser and
  policy registry keys, browser profiles, bookmark and history stores, and
  environment dumps routinely hold the operator's own private URLs. One run
  printed a managed-policy value full of internal admin, CRM, vault and VPN
  links while diagnosing an unrelated browser failure. Prefer a
  narrower probe; if something does spill, stop that line of investigation,
  do not repeat the values anywhere, and report it in the handover.
- **UI checks in a browser (D-041, D-042).** Use
  `scripts/agent-server.ps1 start [-Seed <dir>]`. It prints a localhost URL
  and a `login url:`.
  - Open the login URL in the built-in browser. It signs in as the
    disposable `agent` admin with no password. Use it only on a server this
    script started.
  - Never log in to the Pi, the owner's own server or data, or any other
    host. Never type a password.
  - Finish with `scripts/agent-server.ps1 stop -RunDir <run dir>`, and
    record that in the handover's cleanup section.
  - If a step is refused, report the denial, prove the check with curl and
    component tests instead, and list the browser check as an owner check.
  - Never add `agentdev` to `scripts/build.ps1` or `deploy/` (ADR 0007).
- Do not push, rebase, rewrite history, or delete shared resources unless the
  packet explicitly authorises that exact action.
- Never inspect or edit permission configuration, including any
  `.claude/settings*.json`. When a tool call is denied, report the denial and
  stop that line of work. Do not script around it and do not grant yourself
  permission; adjusting a rule is an owner action. A run that merely *read* a
  settings file to see where a rule was recorded still drew an
  `[Auto-Mode Bypass]` flag.
- Treat a failed live check as evidence. Do not weaken production safeguards or
  substitute a different check to manufacture a pass.
- When Caveman is available, use it for concise chat. Keep code, documentation,
  commit messages and handovers in normal prose.
- When Ponytail is available, use it while designing and implementing. Run
  `ponytail-review` on a non-trivial diff and record its findings. Skill absence
  must not block work; apply the same minimal-change principles directly and
  say so.

### Verify

- Run the checks required by the packet and checks proportionate to the files
  changed. Record exact commands and results.
- Inspect the final diff for scope, secrets, generated files and accidental
  edits.
- Verify every claimed acceptance criterion against code, test output or live
  evidence. Mark unproved criteria incomplete with a dated note.

### Finish

1. Append the handover to the task packet in `../ceems-dev` using its
   existing headings, and commit it there on `main`. Code is committed in the
   task worktree.
2. Commit the task's changes separately in each repository unless the packet
   says not to commit. Use concise Conventional Commits messages.
3. After committing, record final commit hashes in the handover. If this makes
   the handover change, amend only the task's own final commit when safe;
   otherwise create a small documentation commit.
4. Confirm every listed worktree is clean. If inherited or unrelated changes
   prevent that, list every remaining path and do not absorb it into the task.
5. The final chat response contains only the outcome, packet path, commit
   hashes, verification result, and any blocker or cleanup still required.

A handover is incomplete unless it states: outcome, changed files, verification
commands and results, commits, acceptance status, remaining work, and external
resource cleanup.
