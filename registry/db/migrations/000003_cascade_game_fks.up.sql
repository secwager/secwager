ALTER TABLE game_player_stats
  DROP CONSTRAINT game_player_stats_game_id_fkey,
  ADD CONSTRAINT game_player_stats_game_id_fkey
    FOREIGN KEY (game_id) REFERENCES games(id) ON UPDATE CASCADE;
