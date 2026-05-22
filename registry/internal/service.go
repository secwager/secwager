package internal

import (
	"context"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RegistryService implements pb.RegistryServiceServer.
type RegistryService struct {
	pb.UnimplementedRegistryServiceServer
	ref   *refStore
	store InstrumentStore
}

func NewRegistryService(ref *refStore, store InstrumentStore) *RegistryService {
	return &RegistryService{ref: ref, store: store}
}

// ── Reference data ────────────────────────────────────────────────────────────

func (s *RegistryService) GetTeam(_ context.Context, req *pb.GetTeamRequest) (*pb.GetTeamResponse, error) {
	t, ok := s.ref.getTeam(req.Id)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "team %q not found", req.Id)
	}
	return &pb.GetTeamResponse{Team: t}, nil
}

func (s *RegistryService) GetGame(_ context.Context, req *pb.GetGameRequest) (*pb.GetGameResponse, error) {
	g, ok := s.ref.getGame(req.Id)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "game %q not found", req.Id)
	}
	return &pb.GetGameResponse{Game: g}, nil
}

func (s *RegistryService) GetPlayer(_ context.Context, req *pb.GetPlayerRequest) (*pb.GetPlayerResponse, error) {
	p, ok := s.ref.getPlayer(req.Id)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "player %q not found", req.Id)
	}
	return &pb.GetPlayerResponse{Player: p}, nil
}

func (s *RegistryService) ListTeams(_ context.Context, req *pb.ListTeamsRequest) (*pb.ListTeamsResponse, error) {
	return &pb.ListTeamsResponse{Teams: s.ref.listTeams(req.League)}, nil
}

func (s *RegistryService) ListGames(_ context.Context, req *pb.ListGamesRequest) (*pb.ListGamesResponse, error) {
	return &pb.ListGamesResponse{Games: s.ref.listGames(req.League, req.FromUnix, req.ToUnix)}, nil
}

func (s *RegistryService) ListPlayersByGame(_ context.Context, req *pb.ListPlayersByGameRequest) (*pb.ListPlayersByGameResponse, error) {
	return &pb.ListPlayersByGameResponse{Players: s.ref.listPlayersByGame(req.GameId)}, nil
}

func (s *RegistryService) GetAllowedPropTypes(_ context.Context, req *pb.GetAllowedPropTypesRequest) (*pb.GetAllowedPropTypesResponse, error) {
	return &pb.GetAllowedPropTypesResponse{PropTypes: s.ref.allowedPropTypes(req.League, req.Position)}, nil
}

// ── Instruments ───────────────────────────────────────────────────────────────

func (s *RegistryService) CreateInstrument(ctx context.Context, req *pb.CreateInstrumentRequest) (*pb.CreateInstrumentResponse, error) {
	if err := validateStructural(req.Legs); err != nil {
		return nil, err
	}
	if err := validateCrossLeg(req.Legs); err != nil {
		return nil, err
	}
	gameExpiries, err := validateEntities(req.Legs, s.ref)
	if err != nil {
		return nil, err
	}

	id, err := HashLegs(req.Legs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash legs: %v", err)
	}
	expiryUnix := MaxExpiry(req.Legs, gameExpiries)
	expiry := time.Unix(expiryUnix, 0)

	legs, err := collectLegs(req.Legs, s.ref)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "collect legs: %v", err)
	}

	alreadyExisted, err := s.store.Create(ctx, id, expiry, legs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store create: %v", err)
	}
	return &pb.CreateInstrumentResponse{
		InstrumentId:  id,
		ExpiryUnix:    expiryUnix,
		AlreadyExisted: alreadyExisted,
	}, nil
}

func (s *RegistryService) GetInstrument(ctx context.Context, req *pb.GetInstrumentRequest) (*pb.GetInstrumentResponse, error) {
	rec, err := s.store.Get(ctx, req.InstrumentId)
	if err != nil {
		return nil, err
	}
	return &pb.GetInstrumentResponse{Instrument: recordToProto(rec)}, nil
}

func (s *RegistryService) ListInstruments(ctx context.Context, req *pb.ListInstrumentsRequest) (*pb.ListInstrumentsResponse, error) {
	recs, nextToken, err := s.store.List(ctx, ListFilter{
		EntityID:   req.EntityId,
		League:     int32(req.League),
		ActiveOnly: req.ActiveOnly,
		PageSize:   int(req.PageSize),
		PageToken:  req.PageToken,
	})
	if err != nil {
		return nil, err
	}
	instruments := make([]*pb.Instrument, 0, len(recs))
	for _, rec := range recs {
		instruments = append(instruments, recordToProto(rec))
	}
	return &pb.ListInstrumentsResponse{Instruments: instruments, NextPageToken: nextToken}, nil
}

func recordToProto(rec InstrumentRecord) *pb.Instrument {
	return &pb.Instrument{
		InstrumentId: rec.ID,
		Legs:         rec.Legs,
		ExpiryUnix:   rec.ExpiryUnix,
	}
}
