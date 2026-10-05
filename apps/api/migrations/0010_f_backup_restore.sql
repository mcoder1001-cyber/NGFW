CREATE TABLE "f_backup_run" (
	"minute" text PRIMARY KEY NOT NULL,
	"at" timestamp with time zone DEFAULT now() NOT NULL,
	"result" text NOT NULL,
	"filename" text,
	"error" text
);
--> statement-breakpoint
ALTER TABLE "config_candidate" ADD COLUMN "restore_secrets" jsonb;--> statement-breakpoint
CREATE INDEX "f_backup_run_at_idx" ON "f_backup_run" USING btree ("at" DESC NULLS FIRST);