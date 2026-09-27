CREATE TABLE "aaa_external_identity" (
	"user_id" integer PRIMARY KEY NOT NULL,
	"method" text NOT NULL,
	"subject" text NOT NULL,
	"last_groups" text[] DEFAULT '{}'::text[] NOT NULL,
	"last_login" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "aaa_mfa" (
	"user_id" integer PRIMARY KEY NOT NULL,
	"seed" text NOT NULL,
	"enabled" boolean DEFAULT false NOT NULL,
	"last_step" integer DEFAULT 0 NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"enabled_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "aaa_mfa_recovery" (
	"id" serial PRIMARY KEY NOT NULL,
	"user_id" integer NOT NULL,
	"hash" text NOT NULL,
	"used_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "aaa_external_identity" ADD CONSTRAINT "aaa_external_identity_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "aaa_mfa" ADD CONSTRAINT "aaa_mfa_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "aaa_mfa_recovery" ADD CONSTRAINT "aaa_mfa_recovery_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "aaa_external_identity_subject_uq" ON "aaa_external_identity" USING btree ("method","subject");--> statement-breakpoint
CREATE INDEX "aaa_mfa_recovery_user_idx" ON "aaa_mfa_recovery" USING btree ("user_id");