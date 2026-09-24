ALTER TABLE "config_candidate" ADD COLUMN "owner_key_id" uuid;--> statement-breakpoint
ALTER TABLE "config_revision" ADD COLUMN "secret_changes" jsonb DEFAULT '[]'::jsonb NOT NULL;