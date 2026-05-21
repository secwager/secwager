package matchengine

import "strings"

const (
	MaxPrice  = 65536
	MaxOrders = 1 << 20 // 1M slots
)

type orderSlot struct {
	trader    [8]byte
	size      Size
	next      OrderID // next order at same price level; 0 = none
	price     Price
	side      Side
	active    bool
	qtyFilled Size
	fills     []Match
}

type priceLevel struct {
	head OrderID
	tail OrderID
}

const none OrderID = 0

type OrderBook struct {
	pool    [MaxOrders]orderSlot
	poolTop OrderID
	buys    [MaxPrice]priceLevel
	sells   [MaxPrice]priceLevel
	bidMax  uint32 // best bid price; 0 = no bids
	askMin  uint32 // best ask price; MaxPrice = no asks
}

func New() *OrderBook {
	b := &OrderBook{}
	b.reset()
	return b
}

func (b *OrderBook) reset() {
	b.pool = [MaxOrders]orderSlot{}
	b.buys = [MaxPrice]priceLevel{}
	b.sells = [MaxPrice]priceLevel{}
	b.poolTop = 1 // 0 is the NONE sentinel
	b.bidMax = 0
	b.askMin = MaxPrice
}

// Limit submits a limit order. Returns the assigned OrderID and all events generated.
// On rejection, OrderID is none (0).
func (b *OrderBook) Limit(order Order) (OrderID, []Event) {
	if order.Price == 0 {
		return none, []Event{OrderEvent{
			Trader: traderStr(order.Trader),
			Type:   OrderRejected,
			Reason: RejectUnspecified,
		}}
	}
	if b.poolTop >= MaxOrders {
		return none, []Event{OrderEvent{
			Trader: traderStr(order.Trader),
			Type:   OrderRejected,
			Reason: RejectPoolExhausted,
		}}
	}

	id := b.alloc(order)
	prevBid, prevBidQty := b.BestBid(), b.BestBidQty()
	prevAsk, prevAskQty := b.BestAsk(), b.BestAskQty()

	var events []Event
	if order.Side == Buy {
		events = b.matchBuy(id, order, events)
	} else {
		events = b.matchSell(id, order, events)
	}

	events = b.appendBBOIfChanged(events, prevBid, prevBidQty, prevAsk, prevAskQty)
	return id, events
}

func (b *OrderBook) matchBuy(id OrderID, order Order, events []Event) []Event {
	for b.pool[id].size > 0 && b.askMin <= uint32(order.Price) {
		lvl := &b.sells[b.askMin]
		b.skipInactive(lvl)
		if lvl.head == none {
			b.askMin++
			continue
		}
		restingID := lvl.head
		resting := &b.pool[restingID]
		fill := min32(b.pool[id].size, resting.size)
		price := resting.price

		b.pool[id].size -= fill
		b.pool[id].qtyFilled += fill
		b.pool[id].fills = append(b.pool[id].fills, Match{OrderID: restingID, Trader: traderStr(resting.trader), Price: price, Qty: fill})

		resting.size -= fill
		resting.qtyFilled += fill
		resting.fills = append(resting.fills, Match{OrderID: id, Trader: traderStr(b.pool[id].trader), Price: price, Qty: fill})

		// resting-side event (fills snapshot at this moment)
		restFills := copyFills(resting.fills)
		events = append(events, OrderEvent{
			OrderID:      restingID,
			Trader:       traderStr(resting.trader),
			Type:         OrderFilled,
			QtyFilled:    resting.qtyFilled,
			QtyRemaining: resting.size,
			Fills:        restFills,
		})

		// aggressor event (cumulative fills so far)
		aggFills := copyFills(b.pool[id].fills)
		events = append(events, OrderEvent{
			OrderID:      id,
			Trader:       traderStr(b.pool[id].trader),
			Type:         OrderFilled,
			QtyFilled:    b.pool[id].qtyFilled,
			QtyRemaining: b.pool[id].size,
			Fills:        aggFills,
		})

		if resting.size == 0 {
			resting.active = false
			lvl.head = resting.next
			if lvl.head == none {
				lvl.tail = none
				b.askMin++
			}
		}
	}

	if b.pool[id].size > 0 {
		b.enqueue(&b.buys[order.Price], id)
		if uint32(order.Price) > b.bidMax {
			b.bidMax = uint32(order.Price)
		}
		events = append(events, OrderEvent{
			OrderID:      id,
			Trader:       traderStr(b.pool[id].trader),
			Type:         OrderAccepted,
			QtyFilled:    b.pool[id].qtyFilled,
			QtyRemaining: b.pool[id].size,
			Fills:        copyFills(b.pool[id].fills),
		})
	} else {
		b.pool[id].active = false
	}
	return events
}

func (b *OrderBook) matchSell(id OrderID, order Order, events []Event) []Event {
	for b.pool[id].size > 0 && b.bidMax >= uint32(order.Price) && b.bidMax > 0 {
		lvl := &b.buys[b.bidMax]
		b.skipInactive(lvl)
		if lvl.head == none {
			b.bidMax--
			continue
		}
		restingID := lvl.head
		resting := &b.pool[restingID]
		fill := min32(b.pool[id].size, resting.size)
		price := resting.price

		b.pool[id].size -= fill
		b.pool[id].qtyFilled += fill
		b.pool[id].fills = append(b.pool[id].fills, Match{OrderID: restingID, Trader: traderStr(resting.trader), Price: price, Qty: fill})

		resting.size -= fill
		resting.qtyFilled += fill
		resting.fills = append(resting.fills, Match{OrderID: id, Trader: traderStr(b.pool[id].trader), Price: price, Qty: fill})

		// resting-side event
		restFills := copyFills(resting.fills)
		events = append(events, OrderEvent{
			OrderID:      restingID,
			Trader:       traderStr(resting.trader),
			Type:         OrderFilled,
			QtyFilled:    resting.qtyFilled,
			QtyRemaining: resting.size,
			Fills:        restFills,
		})

		// aggressor event (cumulative)
		aggFills := copyFills(b.pool[id].fills)
		events = append(events, OrderEvent{
			OrderID:      id,
			Trader:       traderStr(b.pool[id].trader),
			Type:         OrderFilled,
			QtyFilled:    b.pool[id].qtyFilled,
			QtyRemaining: b.pool[id].size,
			Fills:        aggFills,
		})

		if resting.size == 0 {
			resting.active = false
			lvl.head = resting.next
			if lvl.head == none {
				lvl.tail = none
				if b.bidMax > 0 {
					b.bidMax--
				}
			}
		}
	}

	if b.pool[id].size > 0 {
		b.enqueue(&b.sells[order.Price], id)
		if uint32(order.Price) < b.askMin {
			b.askMin = uint32(order.Price)
		}
		events = append(events, OrderEvent{
			OrderID:      id,
			Trader:       traderStr(b.pool[id].trader),
			Type:         OrderAccepted,
			QtyFilled:    b.pool[id].qtyFilled,
			QtyRemaining: b.pool[id].size,
			Fills:        copyFills(b.pool[id].fills),
		})
	} else {
		b.pool[id].active = false
	}
	return events
}

// Cancel removes a resting order and returns the resulting events.
func (b *OrderBook) Cancel(id OrderID) []Event {
	if id == none || id >= b.poolTop {
		return []Event{OrderEvent{OrderID: id, Type: OrderRejected, Reason: RejectNotFound}}
	}
	slot := &b.pool[id]
	if !slot.active {
		reason := RejectNotFound
		if slot.size == 0 && id < b.poolTop {
			reason = RejectAlreadyFilled
		}
		return []Event{OrderEvent{OrderID: id, Type: OrderRejected, Reason: reason}}
	}

	prevBid, prevBidQty := b.BestBid(), b.BestBidQty()
	prevAsk, prevAskQty := b.BestAsk(), b.BestAskQty()

	remaining := slot.size
	side := slot.side
	price := slot.price
	slot.active = false
	slot.size = 0

	if side == Buy && uint32(price) == b.bidMax {
		b.bidMax = uint32(b.BestBid())
	} else if side == Sell {
		ba := b.BestAsk()
		if ba == 0 {
			b.askMin = MaxPrice
		} else {
			b.askMin = uint32(ba)
		}
	}

	events := []Event{OrderEvent{
		OrderID:      id,
		Trader:       traderStr(slot.trader),
		Type:         OrderCancelled,
		QtyFilled:    slot.qtyFilled,
		QtyRemaining: remaining,
		Fills:        copyFills(slot.fills),
	}}
	return b.appendBBOIfChanged(events, prevBid, prevBidQty, prevAsk, prevAskQty)
}

// Snapshot exports current resting book state including fill history.
func (b *OrderBook) Snapshot() BookState {
	state := BookState{
		BestBid: b.BestBid(),
		BestAsk: b.BestAsk(),
		PoolTop: b.poolTop,
	}
	for p := int(state.BestBid); p > 0; p-- {
		cur := b.buys[p].head
		for cur != none {
			s := &b.pool[cur]
			if s.active {
				state.Orders = append(state.Orders, OrderEntry{
					ID: cur, Trader: s.trader, Price: s.price,
					Size: s.size, Side: Buy,
					QtyFilled: s.qtyFilled, Fills: copyFills(s.fills),
				})
			}
			cur = s.next
		}
	}
	askStart := state.BestAsk
	if askStart == 0 {
		return state
	}
	for p := int(askStart); p < MaxPrice; p++ {
		cur := b.sells[p].head
		for cur != none {
			s := &b.pool[cur]
			if s.active {
				state.Orders = append(state.Orders, OrderEntry{
					ID: cur, Trader: s.trader, Price: s.price,
					Size: s.size, Side: Sell,
					QtyFilled: s.qtyFilled, Fills: copyFills(s.fills),
				})
			}
			cur = s.next
		}
	}
	return state
}

// Restore imports a previously exported BookState.
func (b *OrderBook) Restore(state BookState) {
	b.reset()
	for _, e := range state.Orders {
		if e.ID == none || e.ID >= MaxOrders {
			continue
		}
		slot := &b.pool[e.ID]
		slot.trader = e.Trader
		slot.size = e.Size
		slot.next = none
		slot.price = e.Price
		slot.side = e.Side
		slot.active = true
		slot.qtyFilled = e.QtyFilled
		slot.fills = copyFills(e.Fills)
		if e.Side == Buy {
			b.enqueue(&b.buys[e.Price], e.ID)
		} else {
			b.enqueue(&b.sells[e.Price], e.ID)
		}
	}
	b.bidMax = uint32(state.BestBid)
	if state.BestAsk == 0 {
		b.askMin = MaxPrice
	} else {
		b.askMin = uint32(state.BestAsk)
	}
	b.poolTop = state.PoolTop
	if b.poolTop < 1 {
		b.poolTop = 1
	}
}

func (b *OrderBook) BestBid() Price {
	for p := int64(b.bidMax); p > 0; p-- {
		h := b.buys[p].head
		for h != none && !b.pool[h].active {
			h = b.pool[h].next
		}
		if h != none {
			return Price(p)
		}
	}
	return 0
}

func (b *OrderBook) BestAsk() Price {
	for p := b.askMin; p < MaxPrice; p++ {
		h := b.sells[p].head
		for h != none && !b.pool[h].active {
			h = b.pool[h].next
		}
		if h != none {
			return Price(p)
		}
	}
	return 0
}

func (b *OrderBook) BestBidQty() Size {
	p := b.BestBid()
	if p == 0 {
		return 0
	}
	var qty Size
	for cur := b.buys[p].head; cur != none; cur = b.pool[cur].next {
		if b.pool[cur].active {
			qty += b.pool[cur].size
		}
	}
	return qty
}

func (b *OrderBook) BestAskQty() Size {
	p := b.BestAsk()
	if p == 0 {
		return 0
	}
	var qty Size
	for cur := b.sells[p].head; cur != none; cur = b.pool[cur].next {
		if b.pool[cur].active {
			qty += b.pool[cur].size
		}
	}
	return qty
}

func (b *OrderBook) alloc(o Order) OrderID {
	id := b.poolTop
	b.poolTop++
	slot := &b.pool[id]
	slot.trader = o.Trader
	slot.size = o.Size
	slot.next = none
	slot.price = o.Price
	slot.side = o.Side
	slot.active = true
	slot.qtyFilled = 0
	slot.fills = nil
	return id
}

func (b *OrderBook) enqueue(lvl *priceLevel, id OrderID) {
	b.pool[id].next = none
	if lvl.tail != none {
		b.pool[lvl.tail].next = id
	} else {
		lvl.head = id
	}
	lvl.tail = id
}

func (b *OrderBook) skipInactive(lvl *priceLevel) {
	for lvl.head != none && !b.pool[lvl.head].active {
		lvl.head = b.pool[lvl.head].next
	}
	if lvl.head == none {
		lvl.tail = none
	}
}

func (b *OrderBook) appendBBOIfChanged(events []Event, prevBid Price, prevBidQty Size, prevAsk Price, prevAskQty Size) []Event {
	bid, bidQty := b.BestBid(), b.BestBidQty()
	ask, askQty := b.BestAsk(), b.BestAskQty()
	if bid != prevBid || bidQty != prevBidQty || ask != prevAsk || askQty != prevAskQty {
		events = append(events, BBOEvent{Bid: bid, BidQty: bidQty, Ask: ask, AskQty: askQty})
	}
	return events
}

func copyFills(src []Match) []Match {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Match, len(src))
	copy(dst, src)
	return dst
}

func traderStr(t [8]byte) string {
	return strings.TrimRight(string(t[:]), "\x00")
}

func min32(a, b Size) Size {
	if a < b {
		return a
	}
	return b
}
