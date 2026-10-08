# Resolved source decision: agent daemon ownership and identity paths

The manager selected the implementation on 2026-10-08 under the owner's explicit completion instruction after disclosure of these mismatches. See [the complete decision and security rationale](DEC-agent-file-ownership-20261008.md).

The source change adds CAP_CHOWN and CAP_DAC_OVERRIDE while retaining the strict mount sandbox and explicit writable paths. Public system-identity files use fixed package-provisioned links into a dedicated public state directory; `/etc` is not made broadly writable.

This historical path is retained for existing board/document links. It no longer represents an unanswered architecture choice. Independent review, the final hosted capability/ownership fixtures and actual sandboxed-appliance acceptance must still be recorded before claiming completion. P10's previous installed-development-bundle acceptance remains valid. Product license authority remains separately pending.
