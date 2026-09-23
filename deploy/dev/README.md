# Local dev services

The API's own tests need PostgreSQL 16 and Valkey (P06). On the dev host they run as plain
systemd services (`postgresql`, `valkey-server`) bound to localhost — no containers.
