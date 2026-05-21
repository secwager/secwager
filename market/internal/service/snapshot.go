package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	me "github.com/secwager/secwager/matchengine"
	market_pb "github.com/secwager/secwager/proto/gen/market"
	secwager_pb "github.com/secwager/secwager/proto/gen/secwager"
	"google.golang.org/protobuf/proto"
)

const (
	snapshotFile = "snapshot"
	snapshotTmp  = "snapshot.tmp"
)

func marshalBookState(state me.BookState) *secwager_pb.BookSnapshot {
	snap := &secwager_pb.BookSnapshot{
		BestBid: uint32(state.BestBid),
		BestAsk: uint32(state.BestAsk),
		PoolTop: state.PoolTop,
	}
	for _, e := range state.Orders {
		side := "B"
		if e.Side == me.Sell {
			side = "S"
		}
		os := &secwager_pb.OrderSnapshot{
			Id:        e.ID,
			Trader:    strings.TrimRight(string(e.Trader[:]), "\x00"),
			Price:     uint32(e.Price),
			Size:      e.Size,
			Side:      side,
			QtyFilled: e.QtyFilled,
		}
		for _, m := range e.Fills {
			os.Matches = append(os.Matches, &secwager_pb.MatchSnapshot{
				OrderId: m.OrderID,
				Trader:  m.Trader,
				Price:   uint32(m.Price),
				Qty:     m.Qty,
			})
		}
		snap.Orders = append(snap.Orders, os)
	}
	return snap
}

func unmarshalBookState(pb *secwager_pb.BookSnapshot) me.BookState {
	state := me.BookState{
		BestBid: me.Price(pb.BestBid),
		BestAsk: me.Price(pb.BestAsk),
		PoolTop: pb.PoolTop,
	}
	for _, o := range pb.Orders {
		var t [8]byte
		copy(t[:], o.Trader)
		side := me.Buy
		if o.Side == "S" {
			side = me.Sell
		}
		entry := me.OrderEntry{
			ID:        o.Id,
			Trader:    t,
			Price:     me.Price(o.Price),
			Size:      o.Size,
			Side:      side,
			QtyFilled: o.QtyFilled,
		}
		for _, m := range o.Matches {
			entry.Fills = append(entry.Fills, me.Match{
				OrderID: m.OrderId,
				Trader:  m.Trader,
				Price:   me.Price(m.Price),
				Qty:     m.Qty,
			})
		}
		state.Orders = append(state.Orders, entry)
	}
	return state
}

// persist writes the current book state and offset atomically via tmp+rename.
func persist(dataDir string, offset int64, engines map[string]*me.OrderBook) error {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("mkdir dataDir: %w", err)
	}
	snap := &market_pb.MarketSnapshot{Offset: offset}
	for symbol, eng := range engines {
		state := eng.Snapshot()
		snap.Books = append(snap.Books, &market_pb.SymbolBook{
			Symbol: symbol,
			Book:   marshalBookState(state),
		})
	}
	data, err := proto.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	tmp := filepath.Join(dataDir, snapshotTmp)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write snapshot.tmp: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dataDir, snapshotFile)); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	return nil
}

// load reads the snapshot from disk. Returns offset=-1 and empty books on first boot.
func load(dataDir string) (offset int64, books map[string]me.BookState, err error) {
	books = make(map[string]me.BookState)
	data, readErr := os.ReadFile(filepath.Join(dataDir, snapshotFile))
	if errors.Is(readErr, fs.ErrNotExist) {
		return -1, books, nil
	}
	if readErr != nil {
		return 0, nil, fmt.Errorf("read snapshot: %w", readErr)
	}
	var snap market_pb.MarketSnapshot
	if err := proto.Unmarshal(data, &snap); err != nil {
		return 0, nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	for _, sb := range snap.Books {
		books[sb.Symbol] = unmarshalBookState(sb.Book)
	}
	return snap.Offset, books, nil
}
