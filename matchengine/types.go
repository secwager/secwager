package matchengine

type OrderID = uint32
type Price = uint16 // 1–65535; binary options
type Size = uint32

type Side byte

const (
	Buy  Side = 'B'
	Sell Side = 'S'
)

type ExecType byte

const (
	OrderAccepted  ExecType = iota // resting in book after submit; Fills empty
	OrderFilled                    // fill occurred; QtyRemaining==0 means fully matched
	OrderCancelled
	OrderRejected
)

type RejectReason int

const (
	RejectUnspecified   RejectReason = iota
	RejectPoolExhausted              // new order: pool full
	RejectAlreadyFilled              // cancel: slot exists but qty=0
	RejectNotFound                   // cancel: ID not in pool
)

type Order struct {
	Trader [8]byte
	Price  Price
	Size   Size
	Side   Side
}

// Event is the sealed interface returned by Limit and Cancel.
// Concrete types: OrderEvent, BBOEvent.
type Event interface{ isEvent() }

// Match is one fill leg — a single resting order that contributed to a fill.
type Match struct {
	OrderID OrderID
	Trader  string
	Price   Price
	Qty     Size
}

// OrderEvent covers all order lifecycle events.
// On a fill, the resting side gets one event with Fills growing if it was previously partial.
// The aggressor gets one event per resting order matched, with Fills accumulating across the sweep.
type OrderEvent struct {
	OrderID      OrderID
	Trader       string
	Type         ExecType
	QtyFilled    Size         // cumulative filled qty; 0 for non-fill events
	QtyRemaining Size
	Fills        []Match      // empty for Accepted/Cancelled/Rejected
	Reason       RejectReason // set for OrderRejected
}

func (OrderEvent) isEvent() {}

// BBOEvent is emitted whenever best bid or ask price/qty changes.
// Not tied to any specific order.
type BBOEvent struct {
	Bid    Price
	BidQty Size
	Ask    Price
	AskQty Size
}

func (BBOEvent) isEvent() {}

// OrderEntry is used in BookState for snapshot/restore.
type OrderEntry struct {
	ID        OrderID
	Trader    [8]byte
	Price     Price
	Size      Size // remaining unfilled qty
	Side      Side
	QtyFilled Size
	Fills     []Match
}

// BookState is returned by Snapshot and consumed by Restore.
type BookState struct {
	Orders  []OrderEntry
	BestBid Price
	BestAsk Price
	PoolTop OrderID // next free ID — preserved to avoid collisions post-restore
}
