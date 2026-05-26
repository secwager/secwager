//go:build integration

package internal

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "refdata",
			"POSTGRES_PASSWORD": "refdata",
			"POSTGRES_DB":       "refdata",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	pgc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}
	defer pgc.Terminate(ctx)

	host, _ := pgc.Host(ctx)
	port, _ := pgc.MappedPort(ctx, "5432/tcp")
	dsn := fmt.Sprintf("host=%s port=%s user=refdata password=refdata dbname=refdata sslmode=disable", host, port.Port())

	// Run migrations from the registry module.
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "registry", "db", "migrations")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration driver: %v\n", err)
		os.Exit(1)
	}
	mg, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration init: %v\n", err)
		os.Exit(1)
	}
	if err := mg.Up(); err != nil && err != migrate.ErrNoChange {
		fmt.Fprintf(os.Stderr, "migration up: %v\n", err)
		os.Exit(1)
	}
	db.Close()

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgxpool: %v\n", err)
		os.Exit(1)
	}
	defer testPool.Close()

	os.Exit(m.Run())
}

// ── Mock HTTP server helpers ───────────────────────────────────────────────────

// mockServer builds an httptest.Server that routes GET requests by matching
// URL path substrings. Routes are checked in order; first match wins.
type mockRoute struct {
	contains string
	body     string
}

func newMockServer(routes []mockRoute) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full := r.URL.String()
		for _, rt := range routes {
			if strings.Contains(full, rt.contains) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(rt.body))
				return
			}
		}
		http.NotFound(w, r)
	}))
}

func resetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{"game_player_stats", "game_lineups", "games", "players", "teams", "seasons", "refdata_fetch_log"}
	for _, tbl := range tables {
		if _, err := testPool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("reset %s: %v", tbl, err)
		}
	}
}

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	testPool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n)
	return n
}

// ── Actual API responses (trimmed) ────────────────────────────────────────────

// EPL fixtures round 1 — fixture 1208021: Man United (33) vs Fulham (36), FT 1-0
// Fetched from v3.football.api-sports.io 2026-05-24
const eplTeamsJSON = `{
  "get": "teams", "parameters": {"league": "39", "season": "2024"},
  "errors": [], "results": 2,
  "response": [
    {"team": {"id": 33, "name": "Manchester United", "code": "MUN"}},
    {"team": {"id": 36, "name": "Fulham", "code": "FUL"}}
  ]
}`

const eplPlayersP1JSON = `{
  "get": "players", "parameters": {"league": "39", "season": "2024", "page": "1"},
  "errors": [], "results": 2,
  "paging": {"current": 1, "total": 1},
  "response": [
    {
      "player": {"id": 526, "name": "André Onana"},
      "statistics": [{"team": {"id": 33}, "games": {"position": "Goalkeeper"}}]
    },
    {
      "player": {"id": 545, "name": "Noussair Mazraoui"},
      "statistics": [{"team": {"id": 33}, "games": {"position": "Defender"}}]
    }
  ]
}`

const eplFixturesR1JSON = `{
  "get": "fixtures", "parameters": {"league": "39", "season": "2024"},
  "errors": [], "results": 1,
  "response": [
    {
      "fixture": {
        "id": 1208021, "timestamp": 1723834800,
        "status": {"short": "FT", "long": "Match Finished", "elapsed": 90}
      },
      "teams": {
        "home": {"id": 33, "name": "Manchester United", "winner": true},
        "away": {"id": 36, "name": "Fulham", "winner": false}
      },
      "goals": {"home": 1, "away": 0}
    }
  ]
}`

// Fixture detail response — used by event fetcher status check
const eplFixtureDetailJSON = `{
  "get": "fixtures", "parameters": {"id": "1208021"},
  "errors": [], "results": 1,
  "response": [
    {
      "fixture": {
        "id": 1208021, "timestamp": 1723834800,
        "status": {"short": "FT", "long": "Match Finished", "elapsed": 90}
      },
      "teams": {"home": {"id": 33}, "away": {"id": 36}},
      "goals": {"home": 1, "away": 0}
    }
  ]
}`

// Fixture player stats — home (Man United): Onana (GK, 2 saves), Maguire (D, 1 yellow)
// Fetched from v3.football.api-sports.io fixture 1208021 team 33
const eplFixturePlayersHomeJSON = `{
  "get": "fixtures/players",
  "errors": [], "results": 1,
  "response": [
    {
      "team": {"id": 33, "name": "Manchester United"},
      "players": [
        {
          "player": {"id": 526, "name": "André Onana"},
          "statistics": [{"games": {"position": "G"}, "goals": {"total": null, "assists": 0, "saves": 2}, "cards": {"yellow": 0}}]
        },
        {
          "player": {"id": 2935, "name": "Harry Maguire"},
          "statistics": [{"games": {"position": "D"}, "goals": {"total": null, "assists": null, "saves": null}, "cards": {"yellow": 1}}]
        }
      ]
    }
  ]
}`

// Fixture player stats — away (Fulham): Ballo-Touré (D, 0 everything)
// Fetched from v3.football.api-sports.io fixture 1208021 team 36
const eplFixturePlayersAwayJSON = `{
  "get": "fixtures/players",
  "errors": [], "results": 1,
  "response": [
    {
      "team": {"id": 36, "name": "Fulham"},
      "players": [
        {
          "player": {"id": 105, "name": "Fodé Ballo-Touré"},
          "statistics": [{"games": {"position": "D"}, "goals": {"total": null, "assists": null, "saves": null}, "cards": {"yellow": 0}}]
        }
      ]
    }
  ]
}`

// MLB responses — OAK vs PIT opening day 2024-03-20, gamePk=745444
// Fetched from statsapi.mlb.com 2026-05-24
const mlbTeamsJSON = `{
  "teams": [
    {"id": 133, "name": "Oakland Athletics", "abbreviation": "OAK"},
    {"id": 134, "name": "Pittsburgh Pirates", "abbreviation": "PIT"}
  ]
}`

const mlbRoster133JSON = `{
  "roster": [
    {"person": {"id": 593428, "fullName": "Brent Rooker"}, "position": {"type": "Outfielder"}},
    {"person": {"id": 506433, "fullName": "Paul Blackburn"}, "position": {"type": "Pitcher"}}
  ]
}`

// gamePk 745444: PIT(away) at OAK(home), Final, away=5 home=2
const mlbScheduleStatusJSON = `{
  "dates": [
    {
      "date": "2024-03-20",
      "games": [
        {
          "gamePk": 745444,
          "gameDate": "2024-03-20T20:10:00Z",
          "gameNumber": 1,
          "status": {"detailedState": "Final"},
          "teams": {
            "home": {"score": 2, "team": {"id": 133}},
            "away": {"score": 5, "team": {"id": 134}}
          }
        }
      ]
    }
  ]
}`

const mlbScheduleJSON = `{
  "dates": [
    {
      "date": "2024-03-20",
      "games": [
        {
          "gamePk": 745444,
          "gameDate": "2024-03-20T20:10:00Z",
          "gameNumber": 1,
          "status": {"detailedState": "Scheduled"},
          "teams": {
            "home": {"score": 0, "team": {"id": 133}},
            "away": {"score": 0, "team": {"id": 134}}
          }
        }
      ]
    }
  ]
}`

// Boxscore for 745444 — Rooker (id=593428): 2H 1RBI; Blackburn (id=506433): 3K 0ER 3BB
const mlbBoxscoreJSON = `{
  "teams": {
    "home": {
      "players": {
        "ID593428": {
          "person": {"id": 593428},
          "stats": {
            "batting":  {"homeRuns": 0, "hits": 2, "rbi": 1, "baseOnBalls": 0},
            "pitching": {}
          }
        },
        "ID506433": {
          "person": {"id": 506433},
          "stats": {
            "batting": {},
            "pitching": {"strikeOuts": 3, "earnedRuns": 0, "baseOnBalls": 3}
          }
        }
      }
    },
    "away": {"players": {}}
  }
}`

// NFL game stats for game 13146 (Bears vs Texans preseason) — trimmed to 2 players
// Fetched from v1.american-football.api-sports.io 2026-05-24
const nflGameStatsJSON = `{
  "get": "games/statistics/players",
  "errors": [], "results": 2,
  "response": [
    {
      "team": {"id": 16, "name": "Chicago Bears"},
      "groups": [
        {
          "name": "Passing",
          "players": [
            {
              "player": {"id": 1999, "name": "Brett Rypien"},
              "statistics": [
                {"name": "yards", "value": "166"},
                {"name": "passing touch downs", "value": "3"},
                {"name": "interceptions", "value": "0"}
              ]
            }
          ]
        },
        {
          "name": "Rushing",
          "players": [
            {
              "player": {"id": 1125, "name": "Khalil Herbert"},
              "statistics": [
                {"name": "yards", "value": "35"},
                {"name": "rushing touch downs", "value": "0"}
              ]
            }
          ]
        },
        {
          "name": "Receiving",
          "players": [
            {
              "player": {"id": 227, "name": "Collin Johnson"},
              "statistics": [
                {"name": "yards", "value": "56"},
                {"name": "receiving touch downs", "value": "2"}
              ]
            }
          ]
        }
      ]
    }
  ]
}`

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSoccerPopulator(t *testing.T) {
	resetDB(t)
	srv := newMockServer([]mockRoute{
		{"/teams", eplTeamsJSON},
		{"/players", eplPlayersP1JSON},
		{"/fixtures/players", eplFixturePlayersHomeJSON}, // shouldn't be called by populator
		{"/fixtures", eplFixturesR1JSON},
	})
	defer srv.Close()

	ap := newTestAPISportsClient("testkey")
	ap.soccerBase = srv.URL
	store := NewStore(testPool)

	if err := FetchSoccerRefData(context.Background(), ap, store, 39, 2024); err != nil {
		t.Fatalf("FetchSoccerRefData: %v", err)
	}

	if got := countRows(t, "teams"); got < 2 {
		t.Errorf("expected ≥2 teams, got %d", got)
	}
	if got := countRows(t, "players"); got < 1 {
		t.Errorf("expected ≥1 players, got %d", got)
	}
	if got := countRows(t, "games"); got < 1 {
		t.Errorf("expected ≥1 games, got %d", got)
	}
	// Verify internal ID format for Man United
	var id string
	testPool.QueryRow(context.Background(), `SELECT id FROM teams WHERE external_vendor='apisports' AND external_id='33'`).Scan(&id)
	if id != "EPL::MUN" {
		t.Errorf("expected EPL::MUN, got %q", id)
	}
}

func TestSoccerEventFetcher(t *testing.T) {
	resetDB(t)

	// Seed Man United and Fulham (needed as FK for the game).
	ctx := context.Background()
	testPool.Exec(ctx, `INSERT INTO seasons (league_id, year) VALUES (3, 2024) ON CONFLICT DO NOTHING`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('EPL::MUN','Manchester United','MUN',3,'33','apisports')`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('EPL::FUL','Fulham','FUL',3,'36','apisports')`)

	// Seed players matching the fixture stats response.
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('EPL::MUN::ONANA','André Onana','EPL::MUN','{10}','526','apisports')`)
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('EPL::MUN::MAGUIRE','Harry Maguire','EPL::MUN','{11}','2935','apisports')`)

	// Seed the game in SCHEDULED state so the event fetcher picks it up.
	testPool.Exec(ctx, `INSERT INTO games (id, league_id, season_year, home_team_id, away_team_id, scheduled_unix, expiry_unix, status, external_id, external_vendor)
		VALUES ('EPL::FUL@MUN::20240816', 3, 2024, 'EPL::MUN', 'EPL::FUL', 1723834800, 1723841400, 'SCHEDULED', '1208021', 'apisports')`)

	srv := newMockServer([]mockRoute{
		{"/fixtures/players?fixture=1208021&team=33", eplFixturePlayersHomeJSON},
		{"/fixtures/players?fixture=1208021&team=36", eplFixturePlayersAwayJSON},
		{"/fixtures", eplFixtureDetailJSON},
	})
	defer srv.Close()

	ap := newTestAPISportsClient("testkey")
	ap.soccerBase = srv.URL

	games := []GameRecord{{
		ID: "EPL::FUL@MUN::20240816", LeagueID: 3, ExternalID: "1208021", ExternalVendor: "apisports",
		HomeTeamID: "EPL::MUN", AwayTeamID: "EPL::FUL", Status: "SCHEDULED",
	}}
	FetchSoccerResults(ctx, ap, NewStore(testPool), games)

	// Game should be marked Final with correct scores.
	var status string
	var homeScore, awayScore int
	testPool.QueryRow(ctx, `SELECT status, home_score, away_score FROM games WHERE id='EPL::FUL@MUN::20240816'`).
		Scan(&status, &homeScore, &awayScore)
	if status != "FINAL" {
		t.Errorf("expected FINAL, got %q", status)
	}
	if homeScore != 1 || awayScore != 0 {
		t.Errorf("expected 1-0, got %d-%d", homeScore, awayScore)
	}

	// Onana should have 2 saves.
	var savesVal int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='EPL::FUL@MUN::20240816' AND player_id='EPL::MUN::ONANA' AND prop_type=22`).
		Scan(&savesVal)
	if savesVal != 2 {
		t.Errorf("expected Onana 2 saves, got %d", savesVal)
	}

	// Maguire should have 1 yellow card.
	var yellowVal int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='EPL::FUL@MUN::20240816' AND player_id='EPL::MUN::MAGUIRE' AND prop_type=23`).
		Scan(&yellowVal)
	if yellowVal != 1 {
		t.Errorf("expected Maguire 1 yellow, got %d", yellowVal)
	}
}

func TestMLBPopulator(t *testing.T) {
	resetDB(t)

	rosterCalled := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		u := r.URL.String()
		switch {
		case strings.Contains(u, "/teams?sportId=1"):
			w.Write([]byte(mlbTeamsJSON))
		case strings.Contains(u, "/roster"):
			rosterCalled++
			w.Write([]byte(mlbRoster133JSON))
		case strings.Contains(u, "/schedule"):
			w.Write([]byte(mlbScheduleJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	mlb := NewMLBClient()
	mlb.base = srv.URL
	store := NewStore(testPool)

	if err := FetchMLBRefData(context.Background(), mlb, store, 2024); err != nil {
		t.Fatalf("FetchMLBRefData: %v", err)
	}

	if got := countRows(t, "teams"); got < 2 {
		t.Errorf("expected ≥2 MLB teams, got %d", got)
	}
	if rosterCalled == 0 {
		t.Error("expected roster endpoint to be called")
	}
	// Verify Oakland internal ID
	var id string
	testPool.QueryRow(context.Background(), `SELECT id FROM teams WHERE external_vendor='mlb_stats' AND external_id='133'`).Scan(&id)
	if id != "MLB::OAK" {
		t.Errorf("expected MLB::OAK, got %q", id)
	}
	// Verify game ID format
	var gameID string
	testPool.QueryRow(context.Background(), `SELECT id FROM games WHERE external_vendor='mlb_stats' AND external_id='745444'`).Scan(&gameID)
	if gameID != "MLB::PIT@OAK::20240320" {
		t.Errorf("expected MLB::PIT@OAK::20240320, got %q", gameID)
	}
}

func TestMLBEventFetcher(t *testing.T) {
	resetDB(t)

	ctx := context.Background()
	testPool.Exec(ctx, `INSERT INTO seasons (league_id, year) VALUES (1, 2024) ON CONFLICT DO NOTHING`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('MLB::OAK','Oakland Athletics','OAK',1,'133','mlb_stats')`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('MLB::PIT','Pittsburgh Pirates','PIT',1,'134','mlb_stats')`)
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('MLB::OAK::ROOKER','Brent Rooker','MLB::OAK','{2}','593428','mlb_stats')`)
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('MLB::OAK::BLACKBURN','Paul Blackburn','MLB::OAK','{1}','506433','mlb_stats')`)
	testPool.Exec(ctx, `INSERT INTO games (id, league_id, season_year, home_team_id, away_team_id, scheduled_unix, expiry_unix, status, external_id, external_vendor)
		VALUES ('MLB::PIT@OAK::20240320', 1, 2024, 'MLB::OAK', 'MLB::PIT', 1710965400, 1710979800, 'SCHEDULED', '745444', 'mlb_stats')`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		u := r.URL.String()
		switch {
		case strings.Contains(u, "gamePks"):
			w.Write([]byte(mlbScheduleStatusJSON))
		case strings.Contains(u, "/boxscore"):
			w.Write([]byte(mlbBoxscoreJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	mlb := NewMLBClient()
	mlb.base = srv.URL

	games := []GameRecord{{
		ID: "MLB::PIT@OAK::20240320", LeagueID: 1, ExternalID: "745444", ExternalVendor: "mlb_stats",
		HomeTeamID: "MLB::OAK", AwayTeamID: "MLB::PIT", Status: "SCHEDULED",
	}}
	FetchMLBResults(ctx, mlb, NewStore(testPool), games)

	var status string
	var homeScore, awayScore int
	testPool.QueryRow(ctx, `SELECT status, home_score, away_score FROM games WHERE id='MLB::PIT@OAK::20240320'`).
		Scan(&status, &homeScore, &awayScore)
	if status != "FINAL" {
		t.Errorf("expected FINAL, got %q", status)
	}
	if homeScore != 2 || awayScore != 5 {
		t.Errorf("expected 2-5, got %d-%d", homeScore, awayScore)
	}

	// Rooker: 2 hits (prop_type=3), 1 RBI (prop_type=4)
	var hits, rbi int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='MLB::PIT@OAK::20240320' AND player_id='MLB::OAK::ROOKER' AND prop_type=3`).Scan(&hits)
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='MLB::PIT@OAK::20240320' AND player_id='MLB::OAK::ROOKER' AND prop_type=4`).Scan(&rbi)
	if hits != 2 {
		t.Errorf("expected Rooker 2 hits, got %d", hits)
	}
	if rbi != 1 {
		t.Errorf("expected Rooker 1 RBI, got %d", rbi)
	}

	// Blackburn: 3 strikeouts (prop_type=2)
	var ks int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='MLB::PIT@OAK::20240320' AND player_id='MLB::OAK::BLACKBURN' AND prop_type=2`).Scan(&ks)
	if ks != 3 {
		t.Errorf("expected Blackburn 3 Ks, got %d", ks)
	}
}

func TestNFLStatsParsing(t *testing.T) {
	resetDB(t)

	ctx := context.Background()
	testPool.Exec(ctx, `INSERT INTO seasons (league_id, year) VALUES (2, 2024) ON CONFLICT DO NOTHING`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('NFL::CHI','Chicago Bears','CHI',2,'16','apisports')`)
	testPool.Exec(ctx, `INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor) VALUES ('NFL::HOU','Houston Texans','HOU',2,'10','apisports')`)
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('NFL::CHI::RYPIEN','Brett Rypien','NFL::CHI','{20}','1999','apisports')`)
	testPool.Exec(ctx, `INSERT INTO players (id, name, team_id, positions, external_id, external_vendor) VALUES ('NFL::CHI::JOHNSON','Collin Johnson','NFL::CHI','{22}','227','apisports')`)
	testPool.Exec(ctx, `INSERT INTO games (id, league_id, season_year, home_team_id, away_team_id, scheduled_unix, expiry_unix, status, external_id, external_vendor)
		VALUES ('NFL::HOU@CHI::20240810', 2, 2024, 'NFL::CHI', 'NFL::HOU', 1723316400, 1723330800, 'SCHEDULED', '13146', 'apisports')`)

	detailJSON := `{"response":[{"game":{"status":{"short":"FT","long":"Finished"}},"scores":{"home":{"total":24},"away":{"total":17}}}]}`

	srv := newMockServer([]mockRoute{
		{"/games/statistics", nflGameStatsJSON},
		{"/games", detailJSON},
	})
	defer srv.Close()

	ap := newTestAPISportsClient("testkey")
	ap.nflBase = srv.URL

	games := []GameRecord{{
		ID: "NFL::HOU@CHI::20240810", LeagueID: 2, ExternalID: "13146", ExternalVendor: "apisports",
		HomeTeamID: "NFL::CHI", AwayTeamID: "NFL::HOU", Status: "SCHEDULED",
	}}
	FetchNFLResults(ctx, ap, NewStore(testPool), games)

	var status string
	testPool.QueryRow(ctx, `SELECT status FROM games WHERE id='NFL::HOU@CHI::20240810'`).Scan(&status)
	if status != "FINAL" {
		t.Errorf("expected FINAL, got %q", status)
	}

	// Rypien: 166 passing yards (prop_type=40), 3 TDs (prop_type=43), 0 INTs (prop_type=44)
	var passingYards, tds, ints int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='NFL::HOU@CHI::20240810' AND player_id='NFL::CHI::RYPIEN' AND prop_type=40`).Scan(&passingYards)
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='NFL::HOU@CHI::20240810' AND player_id='NFL::CHI::RYPIEN' AND prop_type=43`).Scan(&tds)
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='NFL::HOU@CHI::20240810' AND player_id='NFL::CHI::RYPIEN' AND prop_type=44`).Scan(&ints)
	if passingYards != 166 {
		t.Errorf("expected Rypien 166 passing yards, got %d", passingYards)
	}
	if tds != 3 {
		t.Errorf("expected Rypien 3 TDs, got %d", tds)
	}
	if ints != 0 {
		t.Errorf("expected Rypien 0 INTs, got %d", ints)
	}

	// Collin Johnson (WR): 56 receiving yards (prop_type=42), 2 TDs (prop_type=43)
	var recvYards, recvTDs int64
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='NFL::HOU@CHI::20240810' AND player_id='NFL::CHI::JOHNSON' AND prop_type=42`).Scan(&recvYards)
	testPool.QueryRow(ctx, `SELECT value FROM game_player_stats WHERE game_id='NFL::HOU@CHI::20240810' AND player_id='NFL::CHI::JOHNSON' AND prop_type=43`).Scan(&recvTDs)
	if recvYards != 56 {
		t.Errorf("expected Johnson 56 recv yards, got %d", recvYards)
	}
	if recvTDs != 2 {
		t.Errorf("expected Johnson 2 TDs, got %d", recvTDs)
	}
}
