package internal

import (
	"fmt"

	pb "github.com/secwager/secwager/proto/gen/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var soccerLeagues = map[pb.League]bool{
	pb.League_EPL:     true,
	pb.League_LA_LIGA: true,
	pb.League_MLS:     true,
}

// validateStructural performs phase 1 (no I/O) checks.
func validateStructural(legs []*pb.Leg) error {
	if len(legs) == 0 {
		return status.Error(codes.InvalidArgument, "instrument must have at least one leg")
	}
	for i, leg := range legs {
		if leg.GameId == "" {
			return status.Errorf(codes.InvalidArgument, "leg %d: game_id is required", i)
		}
		switch p := leg.Predicate.(type) {
		case *pb.Leg_GameOutcome:
			if p.GameOutcome.Outcome == pb.Outcome_OUTCOME_UNSPECIFIED {
				return status.Errorf(codes.InvalidArgument, "leg %d: outcome must be specified", i)
			}
		case *pb.Leg_PlayerProp:
			pp := p.PlayerProp
			if pp.PlayerId == "" {
				return status.Errorf(codes.InvalidArgument, "leg %d: player_id is required", i)
			}
			if pp.PropType == pb.PropType_PROP_UNSPECIFIED {
				return status.Errorf(codes.InvalidArgument, "leg %d: prop_type must be specified", i)
			}
			if pp.Comparator == pb.Comparator_CMP_UNSPECIFIED {
				return status.Errorf(codes.InvalidArgument, "leg %d: comparator must be specified", i)
			}
			if pp.Threshold < 0 {
				return status.Errorf(codes.InvalidArgument, "leg %d: threshold must be >= 0", i)
			}
			if pp.Comparator == pb.Comparator_GTE && pp.Threshold == 0 {
				return status.Errorf(codes.InvalidArgument, "leg %d: GTE 0 is trivially true", i)
			}
			if pp.Comparator == pb.Comparator_LT && pp.Threshold == 0 {
				return status.Errorf(codes.InvalidArgument, "leg %d: LT 0 is trivially false", i)
			}
		case nil:
			return status.Errorf(codes.InvalidArgument, "leg %d: predicate is required", i)
		}
	}
	return nil
}

// validateCrossLeg performs phase 2 (no I/O) integrity checks across all legs.
func validateCrossLeg(legs []*pb.Leg) error {
	type propKey struct {
		gameID   string
		playerID string
		propType pb.PropType
	}

	seenLegHashes := make(map[string]bool)
	gameOutcomeGames := make(map[string]bool)
	propLegs := make(map[propKey][]*pb.PlayerProp)

	for i, leg := range legs {
		h, err := LegHash(leg)
		if err != nil {
			return fmt.Errorf("hash leg %d: %w", err, err)
		}
		if seenLegHashes[h] {
			return status.Errorf(codes.InvalidArgument, "leg %d: duplicate leg", i)
		}
		seenLegHashes[h] = true

		switch p := leg.Predicate.(type) {
		case *pb.Leg_GameOutcome:
			if gameOutcomeGames[leg.GameId] {
				return status.Errorf(codes.InvalidArgument, "leg %d: contradicting game outcome legs for game %s", i, leg.GameId)
			}
			gameOutcomeGames[leg.GameId] = true
		case *pb.Leg_PlayerProp:
			pp := p.PlayerProp
			key := propKey{gameID: leg.GameId, playerID: pp.PlayerId, propType: pp.PropType}
			propLegs[key] = append(propLegs[key], pp)
		}
	}

	for _, pps := range propLegs {
		if len(pps) < 2 {
			continue
		}
		if err := checkPropContradiction(pps); err != nil {
			return err
		}
	}
	return nil
}

func isLowerBound(c pb.Comparator) bool {
	return c == pb.Comparator_GT || c == pb.Comparator_GTE
}

func isUpperBound(c pb.Comparator) bool {
	return c == pb.Comparator_LT || c == pb.Comparator_LTE
}

func checkPropContradiction(pps []*pb.PlayerProp) error {
	for i := 0; i < len(pps); i++ {
		for j := i + 1; j < len(pps); j++ {
			a, b := pps[i], pps[j]
			// GT a + LTE a or GTE a + LT a → impossible range
			if (a.Comparator == pb.Comparator_GT && b.Comparator == pb.Comparator_LTE && a.Threshold == b.Threshold) ||
				(a.Comparator == pb.Comparator_LTE && b.Comparator == pb.Comparator_GT && a.Threshold == b.Threshold) ||
				(a.Comparator == pb.Comparator_GTE && b.Comparator == pb.Comparator_LT && a.Threshold == b.Threshold) ||
				(a.Comparator == pb.Comparator_LT && b.Comparator == pb.Comparator_GTE && a.Threshold == b.Threshold) {
				return status.Errorf(codes.InvalidArgument, "contradicting prop legs: %v %d and %v %d",
					a.Comparator, a.Threshold, b.Comparator, b.Threshold)
			}
			// EQ with different thresholds
			if a.Comparator == pb.Comparator_EQ && b.Comparator == pb.Comparator_EQ && a.Threshold != b.Threshold {
				return status.Errorf(codes.InvalidArgument, "contradicting prop legs: EQ %d and EQ %d",
					a.Threshold, b.Threshold)
			}
			// Two lower bounds or two upper bounds are degenerate — only the more restrictive is needed
			if isLowerBound(a.Comparator) && isLowerBound(b.Comparator) {
				return status.Errorf(codes.InvalidArgument, "degenerate prop legs: %v %d and %v %d are both lower bounds",
					a.Comparator, a.Threshold, b.Comparator, b.Threshold)
			}
			if isUpperBound(a.Comparator) && isUpperBound(b.Comparator) {
				return status.Errorf(codes.InvalidArgument, "degenerate prop legs: %v %d and %v %d are both upper bounds",
					a.Comparator, a.Threshold, b.Comparator, b.Threshold)
			}
		}
	}
	return nil
}

// validateEntities performs phase 3 (I/O via refStore) entity checks.
// Returns game expiries collected as a side effect (game_id → expiry_unix).
func validateEntities(legs []*pb.Leg, rs *refStore) (gameExpiries map[string]int64, err error) {
	gameExpiries = make(map[string]int64)
	for i, leg := range legs {
		game, ok := rs.getGame(leg.GameId)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "leg %d: game %q not found", i, leg.GameId)
		}
		gameExpiries[leg.GameId] = game.ExpiryUnix

		switch p := leg.Predicate.(type) {
		case *pb.Leg_GameOutcome:
			if p.GameOutcome.Outcome == pb.Outcome_DRAW && !soccerLeagues[game.League] {
				return nil, status.Errorf(codes.InvalidArgument, "leg %d: DRAW outcome only valid for soccer leagues", i)
			}
		case *pb.Leg_PlayerProp:
			pp := p.PlayerProp
			player, ok := rs.getPlayer(pp.PlayerId)
			if !ok {
				return nil, status.Errorf(codes.InvalidArgument, "leg %d: player %q not found", i, pp.PlayerId)
			}
			if !rs.playerInGame(pp.PlayerId, leg.GameId) {
				return nil, status.Errorf(codes.InvalidArgument, "leg %d: player %q is not in game %q", i, pp.PlayerId, leg.GameId)
			}
			if !propAllowedForPlayer(pp.PropType, player, game.League, rs) {
				return nil, status.Errorf(codes.InvalidArgument, "leg %d: prop type %v not allowed for player %q", i, pp.PropType, pp.PlayerId)
			}
		}
	}
	return gameExpiries, nil
}

func propAllowedForPlayer(pt pb.PropType, player *pb.Player, league pb.League, rs *refStore) bool {
	for _, pos := range player.Positions {
		for _, allowed := range rs.allowedPropTypes(league, pos) {
			if allowed == pt {
				return true
			}
		}
	}
	return false
}
