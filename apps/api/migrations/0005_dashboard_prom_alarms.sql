CREATE TABLE "alarm" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"rule" text NOT NULL,
	"instance" text DEFAULT '' NOT NULL,
	"metric" text NOT NULL,
	"severity" text NOT NULL,
	"state" text DEFAULT 'active' NOT NULL,
	"value" text DEFAULT '' NOT NULL,
	"threshold" text DEFAULT '' NOT NULL,
	"message" text DEFAULT '' NOT NULL,
	"raised_at" timestamp with time zone DEFAULT now() NOT NULL,
	"cleared_at" timestamp with time zone,
	"acked_at" timestamp with time zone,
	"acked_by" text
);
--> statement-breakpoint
CREATE UNIQUE INDEX "alarm_active_uq" ON "alarm" USING btree ("rule","instance") WHERE state = 'active';--> statement-breakpoint
CREATE INDEX "alarm_state_idx" ON "alarm" USING btree ("state","raised_at");