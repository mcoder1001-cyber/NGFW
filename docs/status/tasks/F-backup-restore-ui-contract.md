# UI contract for Backup and Upgrade

Admin-only pages `/system/backup-restore` and `/system/upgrade`. All `/api/v1` endpoints use authenticated existing fetch infrastructure. API problem+json on failures. Never persist passphrases in storage.

- POST `/actions/backup`: JSON `{passphrase:string,revisions?:number}` (min12 chars passphrase, 1–1000 revisions default100). Binary `application/octet-stream` download; filename response Content-Disposition `ngfw-backup-<timestamp>.ngfwbackup`.
- POST `/actions/restore`: JSON `{passphrase:string,archive:string}` archive base64 (max32MiB binary); returns `{staged:true,diff:{baseRevision:number|null,changes:[...]}}`. Restore only stages candidate; render diff and use existing pending-change bar/normal commit dialog for commit/confirm.
- GET `/config-templates`: `{items: Record<string,{description:string,parameters:Record<string,{type:'string'|'number'|'boolean',required:boolean}>,patch:Record<string,unknown>}>}`.
- GET `/config-templates/:name`: template above. PUT same path JSON template -> `{staged:true}`; normal commit required for template persistence.
- POST `/config-templates/:name/apply`: `{parameters:Record<string,string|number|boolean>}` -> `{staged:true,diff:{baseRevision,changes}}`.
- Schedules through ordinary candidate PUT `/config/management/backup`: `{enabled:boolean,schedule:string,target?:{type:'local'|'sftp'|'https',path:string,host?:string,port?:number,username?:string,credentialRef?:string,hostKeySha256?:string},retention:number,revisions:number,passphraseRef?:string}`. 5-field UTC cron `*`, `*/n`, integer supported. Read `/config/candidate/management/backup` existing API.
- GET `/state/backup`: `{runs:[{at:string,result:'running'|'success'|'failure',filename?:string,error?:string}]}`.
- POST `/actions/support-bundle`: empty JSON; binary JSON attachment (redacted config/audit/agent summaries).
- POST `/actions/upgrade`: `{op:'status'|'stage'|'activate'|'confirm'|'rollback',bundle?:string}` -> `{lines:string[],done:{summary:string,exitCode:number,stats:Record<string,string>}}`. Status `lines[0]` JSON has documented active_slot/default_slot/versions/pending_slot/staged_slot/confirmed fields (docs/install/ab-upgrade.md).
- POST `/actions/upgrade-upload`: raw `application/vnd.ngfw.update` body streamed (max8GiB), header `x-ngfw-filename: ngfw-update-<major.minor.patch>.tar` -> `{bundle:'/data/updates/<server-generated-safe-name>.tar'}`. Submit returned path with stage. Do not base64 or JSON-wrap upgrade upload. Browser fetch Blob body, upgrade endpoint is separate from backup restore upload.

Own frontend task dir `apps/web/src/domains/system/backup-restore/**`, `locales/en/backup-restore.json`, `locales/fa/backup-restore.json`, anchored router/nav/i18n wiring and frontend tests. Use SchemaForm for template parameter fields where possible. Root API worker owns all backend/proto/schema/agent files. Avoid edits under existing Upgrade implementation (none present).
