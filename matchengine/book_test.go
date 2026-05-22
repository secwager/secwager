package matchengine

import (
	"testing"
)

func trader(s string) [8]byte {
	var b [8]byte
	copy(b[:], s)
	return b
}

func orderEvents(events []Event) []OrderEvent {
	var out []OrderEvent
	for _, e := range events {
		if oe, ok := e.(OrderEvent); ok {
			out = append(out, oe)
		}
	}
	return out
}

func bboEvents(events []Event) []BBOEvent {
	var out []BBOEvent
	for _, e := range events {
		if be, ok := e.(BBOEvent); ok {
			out = append(out, be)
		}
	}
	return out
}

func ofType(events []OrderEvent, t ExecType) []OrderEvent {
	var out []OrderEvent
	for _, e := range events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

func byID(events []OrderEvent, id OrderID) *OrderEvent {
	for i := range events {
		if events[i].OrderID == id {
			return &events[i]
		}
	}
	return nil
}

// --- non-crossing ---

func TestLimit_NonCrossing_Buy(t *testing.T) {
	b := New()
	id, events := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Buy})
	oes := orderEvents(events)
	if len(oes) != 1 {
		t.Fatalf("expected 1 event, got %d", len(oes))
	}
	e := oes[0]
	if e.Type != OrderAccepted || e.OrderID != id || e.QtyRemaining != 10 || e.Trader != "alice" {
		t.Fatalf("unexpected event: %+v", e)
	}
}

func TestLimit_NonCrossing_Sell(t *testing.T) {
	b := New()
	id, events := b.Limit(Order{Trader: trader("bob"), Price: 60, Size: 5, Side: Sell})
	oes := orderEvents(events)
	if len(oes) != 1 {
		t.Fatalf("expected 1 event, got %d", len(oes))
	}
	e := oes[0]
	if e.Type != OrderAccepted || e.OrderID != id || e.QtyRemaining != 5 || e.Trader != "bob" {
		t.Fatalf("unexpected event: %+v", e)
	}
}

// --- full cross ---

func TestLimit_FullCross(t *testing.T) {
	b := New()
	restID, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Sell})

	aggID, events := b.Limit(Order{Trader: trader("bob"), Price: 50, Size: 10, Side: Buy})
	oes := orderEvents(events)
	fills := ofType(oes, OrderFilled)

	if len(fills) != 2 {
		t.Fatalf("expected 2 OrderFilled events, got %d: %+v", len(fills), oes)
	}

	rest := byID(fills, restID)
	if rest == nil || rest.QtyRemaining != 0 || rest.QtyFilled != 10 {
		t.Fatalf("resting side event: %+v", rest)
	}
	if len(rest.Fills) != 1 || rest.Fills[0].Trader != "bob" || rest.Fills[0].Price != 50 || rest.Fills[0].Qty != 10 {
		t.Fatalf("resting fills: %+v", rest.Fills)
	}

	agg := byID(fills, aggID)
	if agg == nil || agg.QtyRemaining != 0 || agg.QtyFilled != 10 {
		t.Fatalf("aggressor event: %+v", agg)
	}
	if len(agg.Fills) != 1 || agg.Fills[0].Trader != "alice" {
		t.Fatalf("aggressor fills: %+v", agg.Fills)
	}

	if len(ofType(oes, OrderAccepted)) != 0 {
		t.Fatal("fully-crossing order should not produce OrderAccepted")
	}
}

// --- partial fill ---

func TestLimit_PartialFill_AggressorLarger(t *testing.T) {
	b := New()
	b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 5, Side: Sell})

	aggID, events := b.Limit(Order{Trader: trader("bob"), Price: 50, Size: 10, Side: Buy})
	oes := orderEvents(events)
	fills := ofType(oes, OrderFilled)
	accepted := ofType(oes, OrderAccepted)

	if len(fills) != 2 {
		t.Fatalf("expected 2 OrderFilled, got %d", len(fills))
	}
	if len(accepted) != 1 {
		t.Fatalf("expected 1 OrderAccepted for remainder, got %d", len(accepted))
	}

	bob := byID(fills, aggID)
	if bob == nil || bob.QtyFilled != 5 || bob.QtyRemaining != 5 {
		t.Fatalf("bob fill: %+v", bob)
	}
	if accepted[0].OrderID != aggID || accepted[0].QtyRemaining != 5 {
		t.Fatalf("bob accepted: %+v", accepted[0])
	}
}

func TestLimit_PartialFill_RestingLarger(t *testing.T) {
	b := New()
	restID, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Sell})

	_, events := b.Limit(Order{Trader: trader("bob"), Price: 50, Size: 3, Side: Buy})
	oes := orderEvents(events)
	fills := ofType(oes, OrderFilled)

	if len(fills) != 2 {
		t.Fatalf("expected 2 OrderFilled, got %d", len(fills))
	}
	rest := byID(fills, restID)
	if rest == nil || rest.QtyRemaining != 7 || rest.QtyFilled != 3 {
		t.Fatalf("resting event: %+v", rest)
	}
}

// --- multi-fill sweep ---

func TestLimit_MultiFill(t *testing.T) {
	b := New()
	b.Limit(Order{Trader: trader("s1"), Price: 50, Size: 3, Side: Sell})
	b.Limit(Order{Trader: trader("s2"), Price: 50, Size: 4, Side: Sell})

	aggID, events := b.Limit(Order{Trader: trader("bob"), Price: 50, Size: 7, Side: Buy})
	oes := orderEvents(events)
	fills := ofType(oes, OrderFilled)

	// s1+bob event, s2+bob event = 4 fill events
	if len(fills) < 4 {
		t.Fatalf("expected ≥4 fill events: %+v", oes)
	}

	if len(ofType(oes, OrderAccepted)) != 0 {
		t.Fatal("fully-crossing order should not produce OrderAccepted")
	}

	// final bob event should show 2 fills (one per resting order)
	var lastBob *OrderEvent
	for i := range fills {
		if fills[i].OrderID == aggID {
			lastBob = &fills[i]
		}
	}
	if lastBob == nil || len(lastBob.Fills) != 2 {
		t.Fatalf("expected bob's final event to have 2 fill legs, got %+v", lastBob)
	}
}

// --- fills accumulate on resting side across multiple crosses ---

func TestLimit_RestingFillAccumulates(t *testing.T) {
	b := New()

	b.Limit(Order{Trader: trader("buy1"), Price: 6, Size: 50, Side: Buy})
	sellID, sellEvents := b.Limit(Order{Trader: trader("seller"), Price: 4, Size: 55, Side: Sell})
	sellOEs := orderEvents(sellEvents)

	sellFills := ofType(sellOEs, OrderFilled)
	sellAccepted := ofType(sellOEs, OrderAccepted)
	if len(sellFills) != 2 || len(sellAccepted) != 1 {
		t.Fatalf("expected 2 fills + 1 accepted for seller, fills=%d accepted=%d", len(sellFills), len(sellAccepted))
	}
	if sellAccepted[0].QtyRemaining != 5 || sellAccepted[0].QtyFilled != 50 {
		t.Fatalf("seller accepted: %+v", sellAccepted[0])
	}

	// second buy crosses the resting sell — seller's fills should grow to 2
	_, events2 := b.Limit(Order{Trader: trader("buy2"), Price: 8, Size: 35, Side: Buy})
	fills2 := ofType(orderEvents(events2), OrderFilled)

	sellerEv := byID(fills2, sellID)
	if sellerEv == nil {
		t.Fatal("no fill event for seller in second match")
	}
	if len(sellerEv.Fills) != 2 {
		t.Fatalf("expected 2 accumulated fills, got %d: %+v", len(sellerEv.Fills), sellerEv.Fills)
	}
	if sellerEv.Fills[0].Trader != "buy1" || sellerEv.Fills[1].Trader != "buy2" {
		t.Fatalf("seller fills order wrong: %+v", sellerEv.Fills)
	}
}

// --- price priority ---

func TestLimit_PricePriority(t *testing.T) {
	b := New()
	b.Limit(Order{Trader: trader("worse"), Price: 51, Size: 5, Side: Sell})
	b.Limit(Order{Trader: trader("best"), Price: 49, Size: 5, Side: Sell})

	aggID, events := b.Limit(Order{Trader: trader("bob"), Price: 55, Size: 5, Side: Buy})
	fills := ofType(orderEvents(events), OrderFilled)

	for _, f := range fills {
		if f.OrderID == aggID {
			if f.Fills[len(f.Fills)-1].Price != 49 {
				t.Fatalf("expected fill at price 49, got %d", f.Fills[len(f.Fills)-1].Price)
			}
		}
	}
	if b.BestAsk() != 51 {
		t.Fatalf("expected best ask 51 after fill, got %d", b.BestAsk())
	}
}

// --- FIFO within price level ---

func TestLimit_FIFOWithinPrice(t *testing.T) {
	b := New()
	firstID, _ := b.Limit(Order{Trader: trader("first"), Price: 50, Size: 5, Side: Sell})
	b.Limit(Order{Trader: trader("second"), Price: 50, Size: 5, Side: Sell})

	_, events := b.Limit(Order{Trader: trader("buyer"), Price: 50, Size: 5, Side: Buy})
	fills := ofType(orderEvents(events), OrderFilled)

	for _, f := range fills {
		if f.OrderID == firstID {
			return
		}
	}
	t.Fatal("FIFO violated: first resting order was not filled first")
}

// --- cancel ---

func TestCancel_RestingOrder(t *testing.T) {
	b := New()
	id, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Buy})

	events := orderEvents(b.Cancel(id))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != OrderCancelled || e.OrderID != id || e.QtyRemaining != 10 {
		t.Fatalf("unexpected cancel event: %+v", e)
	}
	if b.BestBid() != 0 {
		t.Fatalf("expected no bids after cancel, got %d", b.BestBid())
	}
}

func TestCancel_NotFound(t *testing.T) {
	b := New()
	events := orderEvents(b.Cancel(999))
	if len(events) != 1 || events[0].Type != OrderRejected || events[0].Reason != RejectNotFound {
		t.Fatalf("expected RejectNotFound, got %+v", events)
	}
}

func TestCancel_AlreadyFilled(t *testing.T) {
	b := New()
	sellID, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 5, Side: Sell})
	b.Limit(Order{Trader: trader("bob"), Price: 50, Size: 5, Side: Buy})

	events := orderEvents(b.Cancel(sellID))
	if len(events) != 1 || events[0].Type != OrderRejected || events[0].Reason != RejectAlreadyFilled {
		t.Fatalf("expected RejectAlreadyFilled, got %+v", events)
	}
}

// --- BBO ---

func TestBBOChange_FiresOnNewOrder(t *testing.T) {
	b := New()
	_, events := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 5, Side: Buy})
	bbos := bboEvents(events)
	if len(bbos) == 0 {
		t.Fatal("expected BBOEvent")
	}
	if bbos[len(bbos)-1].Bid != 50 {
		t.Fatalf("expected bid=50, got %d", bbos[len(bbos)-1].Bid)
	}
}

func TestBBOChange_FiresOnCancel(t *testing.T) {
	b := New()
	id, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 5, Side: Buy})
	bbos := bboEvents(b.Cancel(id))
	if len(bbos) == 0 {
		t.Fatal("expected BBOEvent on cancel")
	}
}

// --- zero price rejection ---

func TestLimit_ZeroPrice_Rejected(t *testing.T) {
	b := New()
	_, events := b.Limit(Order{Trader: trader("alice"), Price: 0, Size: 10, Side: Buy})
	oes := orderEvents(events)
	if len(oes) != 1 || oes[0].Type != OrderRejected {
		t.Fatalf("expected reject for price=0, got %+v", oes)
	}
}

// --- snapshot / restore ---

func TestSnapshot_Restore_RoundTrip(t *testing.T) {
	b := New()
	b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Buy})
	b.Limit(Order{Trader: trader("bob"), Price: 60, Size: 5, Side: Sell})

	state := b.Snapshot()
	b2 := New()
	b2.Restore(state)

	if b2.BestBid() != 50 || b2.BestAsk() != 60 {
		t.Fatalf("BBO mismatch after restore: bid=%d ask=%d", b2.BestBid(), b2.BestAsk())
	}
	if b2.BestBidQty() != 10 || b2.BestAskQty() != 5 {
		t.Fatalf("qty mismatch: bidQty=%d askQty=%d", b2.BestBidQty(), b2.BestAskQty())
	}
}

func TestSnapshot_Restore_FillHistoryPreserved(t *testing.T) {
	b := New()
	b.Limit(Order{Trader: trader("buy1"), Price: 6, Size: 50, Side: Buy})
	sellID, _ := b.Limit(Order{Trader: trader("seller"), Price: 4, Size: 55, Side: Sell})

	b2 := New()
	b2.Restore(b.Snapshot())

	_, events := b2.Limit(Order{Trader: trader("buy2"), Price: 8, Size: 5, Side: Buy})
	fills := ofType(orderEvents(events), OrderFilled)

	sellerEv := byID(fills, sellID)
	if sellerEv == nil {
		t.Fatal("no fill event for seller after restore")
	}
	if len(sellerEv.Fills) != 2 {
		t.Fatalf("expected 2 accumulated fills after restore, got %d: %+v", len(sellerEv.Fills), sellerEv.Fills)
	}
}

func TestSnapshot_Restore_CancelResolvesCorrectly(t *testing.T) {
	b := New()
	id, _ := b.Limit(Order{Trader: trader("alice"), Price: 50, Size: 10, Side: Buy})

	b2 := New()
	b2.Restore(b.Snapshot())

	events := orderEvents(b2.Cancel(id))
	if len(events) != 1 || events[0].Type != OrderCancelled {
		t.Fatalf("expected OrderCancelled after restore, got %+v", events)
	}
}
