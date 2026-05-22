package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	pb "github.com/secwager/secwager/proto/gen/registry"
	"google.golang.org/protobuf/proto"
)

// HashLegs returns the deterministic instrument ID for the given legs.
// Two calls with the same legs in any order produce the same ID.
func HashLegs(legs []*pb.Leg) (string, error) {
	hashes := make([]string, 0, len(legs))
	for _, leg := range legs {
		data, err := proto.MarshalOptions{Deterministic: true}.Marshal(leg)
		if err != nil {
			return "", fmt.Errorf("marshal leg: %w", err)
		}
		h := sha256.Sum256(data)
		hashes = append(hashes, hex.EncodeToString(h[:]))
	}
	sort.Strings(hashes)
	combined := ""
	for _, h := range hashes {
		combined += h
	}
	final := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(final[:]), nil
}

// LegHash returns the hash of a single leg (used as leg_hash in DB).
func LegHash(leg *pb.Leg) (string, error) {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(leg)
	if err != nil {
		return "", fmt.Errorf("marshal leg: %w", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// MaxExpiry returns the maximum expiry_unix across all legs' games.
func MaxExpiry(legs []*pb.Leg, gameExpiries map[string]int64) int64 {
	var max int64
	for _, leg := range legs {
		if e := gameExpiries[leg.GameId]; e > max {
			max = e
		}
	}
	return max
}
