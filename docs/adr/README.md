# Architecture decision records

One file per decision with lasting design consequences, such as a technology
choice, a data model, an API or file format, or a security boundary.
Task-level choices stay in the packet's **Design decisions** section.

- Copy `0000-template.md` to `NNNN-short-title.md`, using the next free number.
- `packet-architect` drafts it. The coordinator commits it, and the owner
  accepts it when it settles a product or goal question.
- Never rewrite an accepted ADR. Supersede it with a new one and set the old
  one's status to `Superseded by NNNN`.

## Index

| ADR | Title | Status | Date |
|---|---|---|---|
| 0001 | [Stack and runtime shape](0001-stack-and-runtime-shape.md) | Accepted | 2026-10-11 |
| 0002 | [Page storage and output pipeline](0002-page-storage-and-output-pipeline.md) | Accepted | 2026-10-11 |
| 0003 | [Users, permissions and API keys](0003-users-permissions-and-api-keys.md) | Accepted | 2026-10-11 |
| 0004 | [Page write path, versions and concurrency](0004-page-write-path-and-concurrency.md) | Accepted | 2026-10-12 |
| 0005 | [Templates, bindings and scheduled rendering](0005-templates-bindings-and-scheduled-rendering.md) | Accepted | 2026-10-12 |
| 0006 | [Grants, API keys and the REST API contract](0006-grants-api-keys-and-the-rest-contract.md) | Accepted | 2026-10-12 |
| 0007 | [Credential-free agent login](0007-credential-free-agent-login.md) | Accepted | 2026-10-15 |
| 0008 | [The MS-DOS theme: one hand-rolled stylesheet, a self-hosted VGA font, two schemes](0008-ms-dos-theme.md) | Accepted | 2026-10-22 |
| 0009 | [Fastext groups and the index setting own parts of a page's FL lines](0009-fastext-groups.md) | Accepted | 2026-10-24 |
| 0010 | [Releases: a pushed semver tag, a stamped footer, per-platform archives](0010-releases.md) | Accepted | 2026-10-25 |
