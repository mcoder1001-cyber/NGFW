# F-notifications contract recovery

Owner explicitly removed Telegram. Only SMTP (TLS or required STARTTLS) and HTTPS HMAC webhook channels are accepted; secret values remain in the secret store. management.notifications is optional; protobuf ManagementConfig field 7 is additive. NotificationChannel field 4/name telegram reserved permanently. API-owned projection must omit notification configuration from agent desired/drift data. Channels/rules are bounded, names unique and references validated. Live acceptance deferred under owner instruction.
