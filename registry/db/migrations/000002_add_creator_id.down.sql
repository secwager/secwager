DROP INDEX IF EXISTS idx_instruments_creator;
ALTER TABLE instruments DROP COLUMN IF EXISTS creator_id;
