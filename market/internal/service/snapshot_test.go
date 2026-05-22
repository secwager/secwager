package service

import (
	"os"
	"testing"

	me "github.com/secwager/secwager/matchengine"
)

func trader8(s string) [8]byte {
	var b [8]byte
	copy(b[:], s)
	return b
}

func TestSnapshot_FirstBoot(t *testing.T) {
	offset, books, err := load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if offset != -1 {
		t.Fatalf("expected offset=-1 on first boot, got %d", offset)
	}
	if len(books) != 0 {
		t.Fatalf("expected empty books, got %d", len(books))
	}
}

func TestSnapshot_PersistAndLoad(t *testing.T) {
	dir := t.TempDir()

	eng := me.New()
	eng.Limit(me.Order{Trader: trader8("alice"), Price: 50, Size: 10, Side: me.Buy})
	eng.Limit(me.Order{Trader: trader8("bob"), Price: 55, Size: 5, Side: me.Sell})

	if err := persist(dir, 42, map[string]*me.OrderBook{"AAPL": eng}); err != nil {
		t.Fatal(err)
	}

	offset, books, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 42 {
		t.Fatalf("expected offset=42, got %d", offset)
	}
	state, ok := books["AAPL"]
	if !ok {
		t.Fatal("AAPL missing from loaded books")
	}

	eng2 := me.New()
	eng2.Restore(state)

	if eng2.BestBid() != 50 || eng2.BestBidQty() != 10 {
		t.Fatalf("bid mismatch: %d qty %d", eng2.BestBid(), eng2.BestBidQty())
	}
	if eng2.BestAsk() != 55 || eng2.BestAskQty() != 5 {
		t.Fatalf("ask mismatch: %d qty %d", eng2.BestAsk(), eng2.BestAskQty())
	}
}

func TestSnapshot_CancelResolvesAfterRestore(t *testing.T) {
	dir := t.TempDir()

	eng := me.New()
	id, _ := eng.Limit(me.Order{Trader: trader8("alice"), Price: 50, Size: 10, Side: me.Buy})

	if err := persist(dir, 1, map[string]*me.OrderBook{"AAPL": eng}); err != nil {
		t.Fatal(err)
	}
	_, books, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := me.New()
	eng2.Restore(books["AAPL"])

	events := eng2.Cancel(id)
	oes := filterOrderEvents(events)
	if len(oes) != 1 || oes[0].Type != me.OrderCancelled {
		t.Fatalf("expected OrderCancelled after restore, got %+v", oes)
	}
}

func TestSnapshot_FillHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()

	eng := me.New()
	eng.Limit(me.Order{Trader: trader8("buy1"), Price: 6, Size: 50, Side: me.Buy})
	sellID, _ := eng.Limit(me.Order{Trader: trader8("seller"), Price: 4, Size: 55, Side: me.Sell})

	if err := persist(dir, 1, map[string]*me.OrderBook{"X": eng}); err != nil {
		t.Fatal(err)
	}
	_, books, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := me.New()
	eng2.Restore(books["X"])

	_, events := eng2.Limit(me.Order{Trader: trader8("buy2"), Price: 8, Size: 5, Side: me.Buy})
	fills := filterFills(filterOrderEvents(events))

	var sellerEv *me.OrderEvent
	for i := range fills {
		if fills[i].OrderID == sellID {
			sellerEv = &fills[i]
		}
	}
	if sellerEv == nil {
		t.Fatal("no fill event for seller after restore")
	}
	if len(sellerEv.Fills) != 2 {
		t.Fatalf("expected 2 accumulated fills after restore, got %d: %+v", len(sellerEv.Fills), sellerEv.Fills)
	}
	if sellerEv.Fills[0].Trader != "buy1" || sellerEv.Fills[1].Trader != "buy2" {
		t.Fatalf("fill history wrong: %+v", sellerEv.Fills)
	}
}

func TestSnapshot_MultipleSymbols(t *testing.T) {
	dir := t.TempDir()

	aaplEng := me.New()
	aaplEng.Limit(me.Order{Trader: trader8("alice"), Price: 150, Size: 3, Side: me.Buy})

	msftEng := me.New()
	msftEng.Limit(me.Order{Trader: trader8("bob"), Price: 300, Size: 7, Side: me.Sell})

	if err := persist(dir, 99, map[string]*me.OrderBook{"AAPL": aaplEng, "MSFT": msftEng}); err != nil {
		t.Fatal(err)
	}

	offset, books, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 99 || len(books) != 2 {
		t.Fatalf("expected offset=99 and 2 symbols, got %d and %d", offset, len(books))
	}

	e1 := me.New()
	e1.Restore(books["AAPL"])
	if e1.BestBid() != 150 || e1.BestBidQty() != 3 {
		t.Fatalf("AAPL: bid=%d qty=%d", e1.BestBid(), e1.BestBidQty())
	}

	e2 := me.New()
	e2.Restore(books["MSFT"])
	if e2.BestAsk() != 300 || e2.BestAskQty() != 7 {
		t.Fatalf("MSFT: ask=%d qty=%d", e2.BestAsk(), e2.BestAskQty())
	}
}

func TestSnapshot_TmpFileRemovedAfterPersist(t *testing.T) {
	dir := t.TempDir()
	eng := me.New()
	eng.Limit(me.Order{Trader: trader8("alice"), Price: 50, Size: 1, Side: me.Buy})

	if err := persist(dir, 0, map[string]*me.OrderBook{"X": eng}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == snapshotTmp {
			t.Fatal("snapshot.tmp was not removed after persist")
		}
	}
}

func filterOrderEvents(events []me.Event) []me.OrderEvent {
	var out []me.OrderEvent
	for _, e := range events {
		if oe, ok := e.(me.OrderEvent); ok {
			out = append(out, oe)
		}
	}
	return out
}

func filterFills(events []me.OrderEvent) []me.OrderEvent {
	var out []me.OrderEvent
	for _, e := range events {
		if e.Type == me.OrderFilled {
			out = append(out, e)
		}
	}
	return out
}
