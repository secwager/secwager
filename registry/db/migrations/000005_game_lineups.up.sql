CREATE TABLE game_lineups (
    game_id     VARCHAR(255) NOT NULL REFERENCES games(id) ON UPDATE CASCADE ON DELETE CASCADE,
    player_id   VARCHAR(255) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (game_id, player_id)
);
