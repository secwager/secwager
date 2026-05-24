package internal

import pb "github.com/secwager/secwager/proto/gen/registry"

// defaultPropRules returns the static product rules mapping (league, position) → allowed PropTypes.
// These are product-level rules that change only with intentional product decisions.
func defaultPropRules() map[ruleKey][]pb.PropType {
	m := make(map[ruleKey][]pb.PropType)

	// MLB
	m[ruleKey{pb.League_MLB, pb.Position_MLB_BATTER}] = []pb.PropType{
		pb.PropType_HOMERUNS, pb.PropType_HITS, pb.PropType_RBIS, pb.PropType_WALKS,
	}
	m[ruleKey{pb.League_MLB, pb.Position_MLB_PITCHER}] = []pb.PropType{
		pb.PropType_STRIKEOUTS, pb.PropType_EARNED_RUNS, pb.PropType_WALKS,
	}

	// Soccer (EPL, La Liga, MLS — identical rules)
	soccerOutfield := []pb.PropType{pb.PropType_GOALS, pb.PropType_ASSISTS, pb.PropType_YELLOW_CARDS}
	soccerGK := []pb.PropType{pb.PropType_GOALS, pb.PropType_ASSISTS, pb.PropType_SAVES, pb.PropType_YELLOW_CARDS}
	for _, league := range []pb.League{pb.League_EPL, pb.League_LA_LIGA, pb.League_MLS} {
		m[ruleKey{league, pb.Position_SOCCER_FORWARD}] = soccerOutfield
		m[ruleKey{league, pb.Position_SOCCER_MIDFIELDER}] = soccerOutfield
		m[ruleKey{league, pb.Position_SOCCER_DEFENDER}] = soccerOutfield
		m[ruleKey{league, pb.Position_SOCCER_GOALKEEPER}] = soccerGK
	}

	// NFL
	m[ruleKey{pb.League_NFL, pb.Position_NFL_QB}] = []pb.PropType{
		pb.PropType_PASSING_YARDS, pb.PropType_TOUCHDOWNS, pb.PropType_INTERCEPTIONS,
	}
	m[ruleKey{pb.League_NFL, pb.Position_NFL_RB}] = []pb.PropType{
		pb.PropType_RUSHING_YARDS, pb.PropType_TOUCHDOWNS, pb.PropType_RECEIVING_YARDS,
	}
	m[ruleKey{pb.League_NFL, pb.Position_NFL_WR}] = []pb.PropType{
		pb.PropType_RECEIVING_YARDS, pb.PropType_TOUCHDOWNS,
	}
	m[ruleKey{pb.League_NFL, pb.Position_NFL_TE}] = []pb.PropType{
		pb.PropType_RECEIVING_YARDS, pb.PropType_TOUCHDOWNS,
	}

	return m
}
