CREATE TABLE "config_sync" (
	"id" integer PRIMARY KEY NOT NULL,
	"state" text NOT NULL,
	"reason" text DEFAULT '' NOT NULL,
	"txn_id" text,
	"since" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "secret_version" (
	"id" serial PRIMARY KEY NOT NULL,
	"ref" text NOT NULL,
	"version" integer NOT NULL,
	"ciphertext" text NOT NULL,
	"created_by" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "config_revision" ADD COLUMN "secret_versions" jsonb;--> statement-breakpoint
ALTER TABLE "secret" ADD COLUMN "version" integer DEFAULT 1 NOT NULL;--> statement-breakpoint
ALTER TABLE "secret_version" ADD CONSTRAINT "secret_version_created_by_app_user_id_fk" FOREIGN KEY ("created_by") REFERENCES "public"."app_user"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "secret_version_ref_version_uq" ON "secret_version" USING btree ("ref","version");