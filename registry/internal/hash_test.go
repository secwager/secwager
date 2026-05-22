package internal

import (
	"testing"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

func TestHashLegs_orderIndependent(t *testing.T) {
	leg1 := &pb.Leg{GameId: "g1", Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome_HOME_WIN}}}
	leg2 := &pb.Leg{GameId: "g2", Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome_HOME_LOSS}}}

	h1, err := HashLegs([]*pb.Leg{leg1, leg2})
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashLegs([]*pb.Leg{leg2, leg1})
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("expected same hash regardless of order, got %s vs %s", h1, h2)
	}
}

func TestHashLegs_differentLegs(t *testing.T) {
	leg1 := &pb.Leg{GameId: "g1", Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome_HOME_WIN}}}
	leg2 := &pb.Leg{GameId: "g1", Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome_HOME_LOSS}}}

	h1, _ := HashLegs([]*pb.Leg{leg1})
	h2, _ := HashLegs([]*pb.Leg{leg2})
	if h1 == h2 {
		t.Fatal("expected different hashes for different legs")
	}
}

func TestHashLegs_single(t *testing.T) {
	leg := &pb.Leg{GameId: "g1", Predicate: &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome_HOME_WIN}}}
	h, err := HashLegs([]*pb.Leg{leg})
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars", len(h))
	}
}
