package internal

import (
	"context"
	"testing"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Fake store ────────────────────────────────────────────────────────────────

type fakeStore struct {
	instruments map[string]InstrumentRecord
}

func newFakeStore() *fakeStore {
	return &fakeStore{instruments: make(map[string]InstrumentRecord)}
}

func (f *fakeStore) Create(_ context.Context, id string, expiry time.Time, legs []LegRow) (bool, error) {
	if _, ok := f.instruments[id]; ok {
		return true, nil
	}
	// Reconstruct legs from LegRows for Get.
	f.instruments[id] = InstrumentRecord{ID: id, ExpiryUnix: expiry.Unix()}
	return false, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (InstrumentRecord, error) {
	rec, ok := f.instruments[id]
	if !ok {
		return InstrumentRecord{}, status.Errorf(codes.NotFound, "not found")
	}
	return rec, nil
}

func (f *fakeStore) List(_ context.Context, req ListFilter) ([]InstrumentRecord, string, error) {
	var out []InstrumentRecord
	now := time.Now().Unix()
	for _, rec := range f.instruments {
		if req.ActiveOnly && rec.ExpiryUnix <= now {
			continue
		}
		out = append(out, rec)
	}
	return out, "", nil
}

// ── Test fixture ──────────────────────────────────────────────────────────────

// future expiry ensures active_only tests work
const futureExpiry = 9999999999

func testRS() *refStore {
	rs := &refStore{
		teams:      make(map[string]*pb.Team),
		players:    make(map[string]*pb.Player),
		games:      make(map[string]*pb.Game),
		gameRoster: make(map[string][]string),
		propRules:  make(map[ruleKey][]pb.PropType),
	}
	rs.teams["MLB::LAD"] = &pb.Team{Id: "MLB::LAD", League: pb.League_MLB}
	rs.teams["MLB::NYY"] = &pb.Team{Id: "MLB::NYY", League: pb.League_MLB}
	rs.teams["EPL::ARS"] = &pb.Team{Id: "EPL::ARS", League: pb.League_EPL}
	rs.teams["EPL::MCI"] = &pb.Team{Id: "EPL::MCI", League: pb.League_EPL}

	rs.players["JUDGE"] = &pb.Player{Id: "JUDGE", TeamId: "MLB::NYY", Positions: []pb.Position{pb.Position_MLB_BATTER}}
	rs.players["OHTANI"] = &pb.Player{Id: "OHTANI", TeamId: "MLB::LAD", Positions: []pb.Position{pb.Position_MLB_BATTER, pb.Position_MLB_PITCHER}}
	rs.players["HAALAND"] = &pb.Player{Id: "HAALAND", TeamId: "EPL::MCI", Positions: []pb.Position{pb.Position_SOCCER_FORWARD}}

	rs.games["MLB::G1"] = &pb.Game{Id: "MLB::G1", League: pb.League_MLB, HomeTeamId: "MLB::LAD", AwayTeamId: "MLB::NYY", ExpiryUnix: futureExpiry}
	rs.games["EPL::G1"] = &pb.Game{Id: "EPL::G1", League: pb.League_EPL, HomeTeamId: "EPL::ARS", AwayTeamId: "EPL::MCI", ExpiryUnix: futureExpiry}

	rs.gameRoster["MLB::G1"] = []string{"JUDGE", "OHTANI"}
	rs.gameRoster["EPL::G1"] = []string{"HAALAND"}

	rs.propRules[ruleKey{pb.League_MLB, pb.Position_MLB_BATTER}] = []pb.PropType{pb.PropType_HOMERUNS, pb.PropType_HITS, pb.PropType_RBIS, pb.PropType_WALKS}
	rs.propRules[ruleKey{pb.League_MLB, pb.Position_MLB_PITCHER}] = []pb.PropType{pb.PropType_STRIKEOUTS, pb.PropType_EARNED_RUNS, pb.PropType_WALKS}
	rs.propRules[ruleKey{pb.League_EPL, pb.Position_SOCCER_FORWARD}] = []pb.PropType{pb.PropType_GOALS, pb.PropType_ASSISTS, pb.PropType_YELLOW_CARDS}

	return rs
}

func newSvc() (*RegistryService, *fakeStore) {
	store := newFakeStore()
	return NewRegistryService(testRS(), store), store
}

func createReq(legs ...*pb.Leg) *pb.CreateInstrumentRequest {
	return &pb.CreateInstrumentRequest{Legs: legs}
}

func gameOutcomeLeg(gameID string, outcome pb.Outcome) *pb.Leg {
	return &pb.Leg{GameId: gameID, Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: outcome}}}
}

func propLeg(gameID, playerID string, pt pb.PropType, cmp pb.Comparator, threshold int64) *pb.Leg {
	return &pb.Leg{GameId: gameID, Predicate: &pb.Leg_PlayerProp{PlayerProp: &pb.PlayerProp{
		PlayerId: playerID, PropType: pt, Comparator: cmp, Threshold: threshold,
	}}}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestCreateInstrument_gameOutcome(t *testing.T) {
	svc, _ := newSvc()
	resp, err := svc.CreateInstrument(context.Background(), createReq(
		gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_WIN),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AlreadyExisted {
		t.Fatal("expected new instrument")
	}
}

func TestCreateInstrument_playerProp(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_GT, 1),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateInstrument_duplicate(t *testing.T) {
	svc, _ := newSvc()
	req := createReq(gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_WIN))
	svc.CreateInstrument(context.Background(), req)
	resp, err := svc.CreateInstrument(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.AlreadyExisted {
		t.Fatal("expected already_existed=true")
	}
}

func TestCreateInstrument_gameNotFound(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		gameOutcomeLeg("UNKNOWN::G", pb.Outcome_HOME_WIN),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateInstrument_playerNotInGame(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("EPL::G1", "JUDGE", pb.PropType_GOALS, pb.Comparator_GT, 0),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateInstrument_propNotAllowedForPosition(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_GOALS, pb.Comparator_GT, 0),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateInstrument_ohtaniTwoWay(t *testing.T) {
	svc, _ := newSvc()
	// STRIKEOUTS is only allowed for MLB_PITCHER — Ohtani has both positions, should be valid.
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "OHTANI", pb.PropType_STRIKEOUTS, pb.Comparator_GT, 5),
	))
	if err != nil {
		t.Fatalf("expected valid for two-way player, got: %v", err)
	}
}

func TestCreateInstrument_triviallyTrue(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_GTE, 0),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for GTE 0, got %v", err)
	}
}

func TestCreateInstrument_triviallyFalse(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_LT, 0),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for LT 0, got %v", err)
	}
}

func TestCreateInstrument_negativeThreshold(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("EPL::G1", "HAALAND", pb.PropType_GOALS, pb.Comparator_EQ, -1),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for negative threshold, got %v", err)
	}
}

func TestCreateInstrument_duplicateGameOutcomeLegs(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_WIN),
		gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_LOSS),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for contradicting game outcome legs, got %v", err)
	}
}

func TestCreateInstrument_propContradictionGTLTE(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_GT, 2),
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_LTE, 2),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for GT/LTE contradiction, got %v", err)
	}
}

func TestCreateInstrument_propContradictionEQ(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_EQ, 1),
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_EQ, 2),
	))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for EQ contradiction, got %v", err)
	}
}

func TestCreateInstrument_duplicateLeg(t *testing.T) {
	svc, _ := newSvc()
	leg := gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_WIN)
	_, err := svc.CreateInstrument(context.Background(), createReq(leg, leg))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for duplicate leg, got %v", err)
	}
}

func TestCreateInstrument_twoLegsForSameGameDifferentPlayers(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_GT, 1),
		propLeg("MLB::G1", "OHTANI", pb.PropType_HITS, pb.Comparator_GT, 1),
	))
	if err != nil {
		t.Fatalf("expected valid multi-player instrument, got: %v", err)
	}
}

func TestCreateInstrument_multiLegValid(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateInstrument(context.Background(), createReq(
		gameOutcomeLeg("MLB::G1", pb.Outcome_HOME_WIN),
		propLeg("MLB::G1", "JUDGE", pb.PropType_HOMERUNS, pb.Comparator_GT, 0),
	))
	if err != nil {
		t.Fatalf("expected valid multi-leg instrument, got: %v", err)
	}
}

func TestListInstruments_activeOnly(t *testing.T) {
	svc, store := newSvc()
	// active instrument (future expiry)
	store.instruments["active"] = InstrumentRecord{ID: "active", ExpiryUnix: futureExpiry}
	// expired instrument (past expiry)
	store.instruments["expired"] = InstrumentRecord{ID: "expired", ExpiryUnix: 1}

	resp, err := svc.ListInstruments(context.Background(), &pb.ListInstrumentsRequest{ActiveOnly: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, inst := range resp.Instruments {
		if inst.InstrumentId == "expired" {
			t.Fatal("active_only should exclude expired instruments")
		}
	}
}
