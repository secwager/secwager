package internal

import (
	"fmt"
	"strings"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// soccerLeagueMap maps api-sports football league ID → proto enum.
var soccerLeagueMap = map[int]pb.League{
	39:  pb.League_EPL,
	140: pb.League_LA_LIGA,
	253: pb.League_MLS,
}

// soccerPlayerListPositionMap maps the position string from /players (season listing) → proto enum.
// Actual values confirmed from live API: "Attacker", "Defender", "Midfielder".
// Goalkeeper variants included for safety.
var soccerPlayerListPositionMap = map[string]pb.Position{
	"Goalkeeper": pb.Position_SOCCER_GOALKEEPER,
	"Keeper":     pb.Position_SOCCER_GOALKEEPER,
	"Defender":   pb.Position_SOCCER_DEFENDER,
	"Midfielder": pb.Position_SOCCER_MIDFIELDER,
	"Attacker":   pb.Position_SOCCER_FORWARD,
	"Forward":    pb.Position_SOCCER_FORWARD,
}

// soccerFixturePositionMap maps the single-letter position code from /fixtures/players → proto enum.
// Confirmed from live API: "G", "D", "M", "F".
var soccerFixturePositionMap = map[string]pb.Position{
	"G": pb.Position_SOCCER_GOALKEEPER,
	"D": pb.Position_SOCCER_DEFENDER,
	"M": pb.Position_SOCCER_MIDFIELDER,
	"F": pb.Position_SOCCER_FORWARD,
}

// leaguePrefix is the string prefix used in internal IDs.
var leaguePrefix = map[pb.League]string{
	pb.League_MLB:     "MLB",
	pb.League_NFL:     "NFL",
	pb.League_EPL:     "EPL",
	pb.League_LA_LIGA: "LA_LIGA",
	pb.League_MLS:     "MLS",
}

func makeTeamID(league pb.League, code string) string {
	return leaguePrefix[league] + "::" + strings.ToUpper(code)
}

func makePlayerID(league pb.League, teamCode, fullName string) string {
	parts := strings.Fields(fullName)
	lastName := strings.ToUpper(parts[len(parts)-1])
	return leaguePrefix[league] + "::" + strings.ToUpper(teamCode) + "::" + lastName
}

// makeGameID constructs the internal game ID. For MLB doubleheaders, pass gameNumber > 1
// to append the _G{n} suffix.
func makeGameID(league pb.League, awayCode, homeCode string, scheduledUnix int64, gameNumber int) string {
	t := time.Unix(scheduledUnix, 0).UTC()
	base := fmt.Sprintf("%s::%s@%s::%s",
		leaguePrefix[league],
		strings.ToUpper(awayCode),
		strings.ToUpper(homeCode),
		t.Format("20060102"),
	)
	if gameNumber > 1 {
		return fmt.Sprintf("%s_G%d", base, gameNumber)
	}
	return base
}

// nflPositionMap maps api-sports american-football position string → proto enum.
// Positions not listed here (OL, K, P, etc.) are skipped — no props for them.
var nflPositionMap = map[string]pb.Position{
	"QB":  pb.Position_NFL_QB,
	"RB":  pb.Position_NFL_RB,
	"WR":  pb.Position_NFL_WR,
	"TE":  pb.Position_NFL_TE,
	"DEF": pb.Position_NFL_DEF,
}

// mlbPositionMap maps MLB Stats API position.type → proto enum.
var mlbPositionMap = map[string]pb.Position{
	"Pitcher":   pb.Position_MLB_PITCHER,
	"Outfielder": pb.Position_MLB_BATTER,
	"Infielder":  pb.Position_MLB_BATTER,
	"Catcher":    pb.Position_MLB_BATTER,
	"Hitter":     pb.Position_MLB_BATTER,
	"Two-Way Player": pb.Position_MLB_BATTER, // Ohtani-type; pitcher position added separately
}
