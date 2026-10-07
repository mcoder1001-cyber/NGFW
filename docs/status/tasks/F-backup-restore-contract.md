# F-backup-restore additive contract

Schema source: packages/schema/src/domains/ext/backup-restore.ts. Management adds optional backup configuration and templates. Existing protobuf field numbers remain unchanged: notifications retains 7, backup uses 8, templates uses 9. ActionRequest adds upgrade at 20 and support_bundle at 21. UpgradeOp is a closed status/stage/activate/confirm/rollback enum; no client shell command is accepted.

Generated migration 0010_f_backup_restore.sql adds nullable candidate restore_secrets metadata and the durable f_backup_run table. Restored encrypted versions remain inactive until normal commit; discard drops candidate pins. Account definitions and app_user password hashes remain those of the current appliance.

Public template DTOs accept patch as a JSON object. The mirrored persistent schema/protobuf uses patchJson, a validated JSON string, because arbitrary Struct maps violate the project's schema drift contract. Controller/service conversion is explicit; safe prototype keys and inline credentials are rejected. Whole-value ${parameter} placeholders support typed substitution only.

REST endpoints are POST actions/backup, actions/restore, actions/support-bundle, actions/upgrade; POST actions/upgrade-upload accepts streaming application/vnd.ngfw.update; GET state/backup; GET and PUT config-templates; POST config-templates/:name/apply, all under /api/v1. Successful restore/apply output includes staged and diff. Upgrade output includes operation-specific status. Generated OpenAPI, TypeScript client and CLI operations table carry these additive endpoints; the CLI does not introduce a separate interactive wizard.

Schedules use normal GET /config/candidate/management/backup and PUT /config/management/backup, followed by ordinary validation/commit. Local/SFTP retention is performed by the appliance. HTTPS sends a unique filename and retention header to a receiver that must implement retention. Dedicated root ngfw-upgrade@.service instances perform fixed operations; the agent's existing sandbox remains intact.

Run history index f_backup_run_at_idx uses at DESC NULLS FIRST, matching PostgreSQL ORDER BY at DESC. Generated single task migration0010 and its snapshot/journal were regenerated from unchanged baseline0009; no second migration or handwritten schema snapshot.
