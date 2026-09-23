CREATE TABLE "api_key" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"user_id" integer NOT NULL,
	"name" text NOT NULL,
	"hash" text NOT NULL,
	"scopes" text[] DEFAULT '{}'::text[] NOT NULL,
	"expires_at" timestamp with time zone,
	"last_used" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "app_user" (
	"id" serial PRIMARY KEY NOT NULL,
	"username" text NOT NULL,
	"password_hash" text,
	"role" text NOT NULL,
	"mfa_secret" text,
	"disabled" boolean DEFAULT false NOT NULL,
	"last_login" timestamp with time zone,
	"failed_logins" integer DEFAULT 0 NOT NULL,
	"locked_until" timestamp with time zone,
	"source" text DEFAULT 'config' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "app_user_role_ck" CHECK ("app_user"."role" in ('admin', 'operator', 'readonly'))
);
--> statement-breakpoint
CREATE TABLE "audit_log" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"ts" timestamp with time zone DEFAULT now() NOT NULL,
	"user_id" integer,
	"username" text,
	"source_ip" text,
	"action" text NOT NULL,
	"resource" text,
	"before" jsonb,
	"after" jsonb,
	"result" text NOT NULL,
	"status" integer
);
--> statement-breakpoint
CREATE TABLE "config_candidate" (
	"id" integer PRIMARY KEY NOT NULL,
	"owner_id" integer,
	"locked_at" timestamp with time zone,
	"payload" jsonb,
	"base_revision_id" integer,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "config_pending" (
	"id" integer PRIMARY KEY NOT NULL,
	"txn_id" text NOT NULL,
	"payload" jsonb NOT NULL,
	"hash" text NOT NULL,
	"author_id" integer,
	"comment" text DEFAULT '' NOT NULL,
	"parent_id" integer,
	"kind" text NOT NULL,
	"deadline" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "config_revision" (
	"id" serial PRIMARY KEY NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"author_id" integer,
	"comment" text DEFAULT '' NOT NULL,
	"parent_id" integer,
	"payload" jsonb NOT NULL,
	"hash" text NOT NULL,
	"txn_id" text,
	"kind" text DEFAULT 'commit' NOT NULL
);
--> statement-breakpoint
CREATE TABLE "secret" (
	"id" serial PRIMARY KEY NOT NULL,
	"kind" text NOT NULL,
	"ref" text NOT NULL,
	"ciphertext" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "system_event" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"ts" timestamp with time zone DEFAULT now() NOT NULL,
	"severity" text NOT NULL,
	"subsystem" text NOT NULL,
	"code" text NOT NULL,
	"message" text NOT NULL,
	"data" jsonb
);
--> statement-breakpoint
ALTER TABLE "api_key" ADD CONSTRAINT "api_key_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_log" ADD CONSTRAINT "audit_log_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "config_candidate" ADD CONSTRAINT "config_candidate_owner_id_app_user_id_fk" FOREIGN KEY ("owner_id") REFERENCES "public"."app_user"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "config_pending" ADD CONSTRAINT "config_pending_author_id_app_user_id_fk" FOREIGN KEY ("author_id") REFERENCES "public"."app_user"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "config_revision" ADD CONSTRAINT "config_revision_author_id_app_user_id_fk" FOREIGN KEY ("author_id") REFERENCES "public"."app_user"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "api_key_hash_uq" ON "api_key" USING btree ("hash");--> statement-breakpoint
CREATE INDEX "api_key_user_idx" ON "api_key" USING btree ("user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "app_user_username_uq" ON "app_user" USING btree ("username");--> statement-breakpoint
CREATE INDEX "audit_log_ts_idx" ON "audit_log" USING btree ("ts");--> statement-breakpoint
CREATE INDEX "config_revision_created_idx" ON "config_revision" USING btree ("created_at");--> statement-breakpoint
CREATE UNIQUE INDEX "secret_ref_uq" ON "secret" USING btree ("ref");--> statement-breakpoint
CREATE INDEX "system_event_ts_idx" ON "system_event" USING btree ("ts");