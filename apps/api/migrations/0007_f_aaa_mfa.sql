CREATE TABLE "user_mfa_recovery" (
	"id" serial PRIMARY KEY NOT NULL,
	"user_id" integer NOT NULL,
	"code_hash" text NOT NULL,
	"used_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "app_user" ADD COLUMN "mfa_pending_secret" text;--> statement-breakpoint
ALTER TABLE "app_user" ADD COLUMN "mfa_enrolled_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "user_mfa_recovery" ADD CONSTRAINT "user_mfa_recovery_user_id_app_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."app_user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "user_mfa_recovery_user_hash_uq" ON "user_mfa_recovery" USING btree ("user_id","code_hash");--> statement-breakpoint
CREATE INDEX "user_mfa_recovery_user_idx" ON "user_mfa_recovery" USING btree ("user_id");