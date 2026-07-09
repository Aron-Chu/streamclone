# Archived ingest shadow scripts

Ingest-core shadow compare/inspect tooling moved out of the public Streamclone core workflow during the Step 7 boundary split.

Operator copies and production gates live in **private streampulse-ops**. Legacy ingest shadow implementation remains under `internal/analytics/ingestcore/` until the final deletion PR.

Scripts in this folder are reference-only; they are not invoked by `make up`, `make smoke`, or `AGENTS.md` core paths.
