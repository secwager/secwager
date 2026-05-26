ALTER TABLE instruments ADD COLUMN creator_id VARCHAR(255);
CREATE INDEX idx_instruments_creator ON instruments(creator_id);
DELETE FROM instruments WHERE creator_id IS NULL;
