-- Retire Emote Atlas tables and routes; public hub keeps aggregate emote intel only.
DROP TABLE IF EXISTS emote_atlas_materialization_runs;
DROP TABLE IF EXISTS emote_culture_sections;
DROP TABLE IF EXISTS public_emote_rising_snapshots;
DROP TABLE IF EXISTS public_emote_creator_rank_snapshots;
