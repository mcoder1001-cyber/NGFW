CREATE TABLE "auto_block" (
	"id" bigserial PRIMARY KEY NOT NULL,
	"source" text NOT NULL,
	"reason" text NOT NULL,
	"hits" integer DEFAULT 0 NOT NULL,
	"offences" integer DEFAULT 1 NOT NULL,
	"origin" text DEFAULT 'auto' NOT NULL,
	"note" text DEFAULT '' NOT NULL,
	"first_seen" timestamp with time zone DEFAULT now() NOT NULL,
	"blocked_at" timestamp with time zone DEFAULT now() NOT NULL,
	"expires_at" timestamp with time zone NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX "auto_block_source_uq" ON "auto_block" USING btree ("source");--> statement-breakpoint
CREATE INDEX "auto_block_expires_idx" ON "auto_block" USING btree ("expires_at");