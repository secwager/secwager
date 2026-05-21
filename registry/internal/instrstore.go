package internal

import (
	"context"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// LegRow is the DB representation of a single instrument leg.
type LegRow struct {
	LegHash    string
	GameID     string
	League     int32
	Entities   []string // all entity IDs for this leg; populated from refdata at creation time
	Outcome    *int32   // nil for player props
	PlayerID   *string  // nil for game outcomes
	PropType   *int32
	Comparator *int32
	Threshold  *int64
	ExpiryUnix int64
}

// InstrumentRecord is what the store returns for a Get or List.
type InstrumentRecord struct {
	ID         string
	Legs       []*pb.Leg
	ExpiryUnix int64
}

// ListFilter mirrors ListInstrumentsRequest for the store layer.
type ListFilter struct {
	EntityID   string
	League     int32
	ActiveOnly bool
	PageSize   int
	PageToken  string
}

// InstrumentStore is the persistence interface for instruments.
type InstrumentStore interface {
	Create(ctx context.Context, id string, expiry time.Time, legs []LegRow) (alreadyExisted bool, err error)
	Get(ctx context.Context, id string) (InstrumentRecord, error)
	List(ctx context.Context, req ListFilter) (records []InstrumentRecord, nextPageToken string, err error)
}

// collectLegs converts proto legs into LegRow slice, enriching with entity IDs from refdata.
func collectLegs(legs []*pb.Leg, rs *refStore) ([]LegRow, error) {
	rows := make([]LegRow, 0, len(legs))
	for _, leg := range legs {
		h, err := LegHash(leg)
		if err != nil {
			return nil, err
		}
		game, _ := rs.getGame(leg.GameId)
		row := LegRow{
			LegHash:    h,
			GameID:     leg.GameId,
			League:     int32(game.League),
			ExpiryUnix: game.ExpiryUnix,
		}
		// Entities always include the game and both teams.
		row.Entities = append(row.Entities, leg.GameId, game.HomeTeamId, game.AwayTeamId)

		switch p := leg.Predicate.(type) {
		case *pb.Leg_GameOutcome:
			v := int32(p.GameOutcome.Outcome)
			row.Outcome = &v
		case *pb.Leg_PlayerProp:
			pp := p.PlayerProp
			player, _ := rs.getPlayer(pp.PlayerId)
			row.PlayerID = &pp.PlayerId
			propType := int32(pp.PropType)
			row.PropType = &propType
			cmp := int32(pp.Comparator)
			row.Comparator = &cmp
			row.Threshold = &pp.Threshold
			row.Entities = append(row.Entities, pp.PlayerId, player.TeamId)
		}

		rows = append(rows, row)
	}
	return rows, nil
}
