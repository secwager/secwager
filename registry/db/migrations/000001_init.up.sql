CREATE TABLE instruments (
    id         VARCHAR(64) PRIMARY KEY,
    expiry     TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE instrument_legs (
    id            BIGSERIAL   PRIMARY KEY,
    instrument_id VARCHAR(64) NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    leg_hash      VARCHAR(64) NOT NULL,
    game_id       VARCHAR(64) NOT NULL,
    league        INTEGER     NOT NULL,
    outcome       INTEGER,
    player_id     VARCHAR(64),
    prop_type     INTEGER,
    comparator    INTEGER,
    threshold     BIGINT,
    expiry_unix   BIGINT      NOT NULL,
    UNIQUE (instrument_id, leg_hash)
);

-- Every leg links to all entities it involves.
-- predicate=game_outcome: {game_id, home_team_id, away_team_id}
-- predicate=player_prop:  {game_id, player_id, player.team_id, home_team_id, away_team_id}
CREATE TABLE instrument_leg_entities (
    leg_id        BIGINT      NOT NULL REFERENCES instrument_legs(id) ON DELETE CASCADE,
    instrument_id VARCHAR(64) NOT NULL REFERENCES instruments(id)     ON DELETE CASCADE,
    entity_id     VARCHAR(64) NOT NULL,
    PRIMARY KEY (leg_id, entity_id)
);

CREATE INDEX idx_legs_instrument         ON instrument_legs(instrument_id);
CREATE INDEX idx_legs_game               ON instrument_legs(game_id);
CREATE INDEX idx_legs_expiry             ON instrument_legs(expiry_unix);
CREATE INDEX idx_leg_entities            ON instrument_leg_entities(entity_id);
CREATE INDEX idx_leg_entities_instrument ON instrument_leg_entities(instrument_id);
