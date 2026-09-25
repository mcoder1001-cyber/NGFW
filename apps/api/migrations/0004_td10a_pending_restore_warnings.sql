ALTER TABLE "config_pending" ADD COLUMN "restore_secrets" jsonb;--> statement-breakpoint
ALTER TABLE "config_pending" ADD COLUMN "warnings" jsonb;