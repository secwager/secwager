CREATE TABLE leagues (
  id              INTEGER     PRIMARY KEY,
  name            VARCHAR(64) NOT NULL,
  short_name      VARCHAR(16) NOT NULL UNIQUE,
  sport           VARCHAR(32) NOT NULL,
  external_id     VARCHAR(64),
  external_vendor VARCHAR(32),
  UNIQUE (external_vendor, external_id)
);

INSERT INTO leagues (id, name, short_name, sport, external_id, external_vendor) VALUES
  (1, 'Major League Baseball',    'MLB',     'baseball',          '1',   'mlb_stats'),
  (2, 'National Football League', 'NFL',     'american_football', '1',   'apisports'),
  (3, 'English Premier League',   'EPL',     'soccer',            '39',  'apisports'),
  (4, 'La Liga',                  'LA_LIGA', 'soccer',            '140', 'apisports'),
  (5, 'Major League Soccer',      'MLS',     'soccer',            '253', 'apisports');

CREATE TABLE seasons (
  league_id  INTEGER NOT NULL REFERENCES leagues(id),
  year       INTEGER NOT NULL,
  PRIMARY KEY (league_id, year)
);

CREATE TABLE teams (
  id              VARCHAR(64) PRIMARY KEY,
  name            TEXT        NOT NULL,
  short_name      TEXT        NOT NULL,
  league_id       INTEGER     NOT NULL REFERENCES leagues(id),
  external_id     VARCHAR(64),
  external_vendor VARCHAR(32),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (external_vendor, external_id)
);

CREATE TABLE players (
  id              VARCHAR(64) PRIMARY KEY,
  name            TEXT        NOT NULL,
  team_id         VARCHAR(64) NOT NULL REFERENCES teams(id),
  positions       INTEGER[]   NOT NULL,
  external_id     VARCHAR(64),
  external_vendor VARCHAR(32),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (external_vendor, external_id)
);

CREATE TABLE games (
  id              VARCHAR(64) PRIMARY KEY,
  league_id       INTEGER     NOT NULL REFERENCES leagues(id),
  season_year     INTEGER     NOT NULL,
  home_team_id    VARCHAR(64) NOT NULL REFERENCES teams(id),
  away_team_id    VARCHAR(64) NOT NULL REFERENCES teams(id),
  scheduled_unix  BIGINT      NOT NULL,
  expiry_unix     BIGINT      NOT NULL,
  status          VARCHAR(16) NOT NULL DEFAULT 'SCHEDULED',
  home_score      INTEGER,
  away_score      INTEGER,
  external_id     VARCHAR(64),
  external_vendor VARCHAR(32),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (league_id, season_year) REFERENCES seasons(league_id, year),
  UNIQUE (external_vendor, external_id)
);

CREATE TABLE game_player_stats (
  game_id     VARCHAR(64) NOT NULL REFERENCES games(id),
  player_id   VARCHAR(64) NOT NULL REFERENCES players(id),
  prop_type   INTEGER     NOT NULL,
  value       BIGINT      NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id, prop_type)
);

CREATE TABLE instruments (
    id          VARCHAR(64) PRIMARY KEY,
    expiry      TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    settled_at  TIMESTAMPTZ,
    settled_win BOOLEAN
);

CREATE TABLE instrument_legs (
    id             BIGSERIAL   PRIMARY KEY,
    instrument_id  VARCHAR(64) NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    leg_hash       VARCHAR(64) NOT NULL,
    game_id        VARCHAR(64) NOT NULL REFERENCES games(id),
    league         INTEGER     NOT NULL,
    outcome        INTEGER,
    player_id      VARCHAR(64) REFERENCES players(id),
    prop_type      INTEGER,
    comparator     INTEGER,
    threshold      BIGINT,
    expiry_unix    BIGINT      NOT NULL,
    settled_result BOOLEAN,
    settled_at     TIMESTAMPTZ,
    UNIQUE (instrument_id, leg_hash)
);

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
