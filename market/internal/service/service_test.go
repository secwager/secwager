package service

import (
	"testing"

	secwager_pb "github.com/secwager/secwager/proto/gen/secwager"
	"google.golang.org/protobuf/proto"
)

type published struct {
	topic   string
	key     string
	payload []byte
}

func newTestService() (*MarketService, *[]published) {
	var msgs []published
	svc := New(Config{
		ExecTopic:  "order_executions",
		DepthTopic: "depth_updates",
		Publisher: func(topic, key string, payload []byte) {
			msgs = append(msgs, published{topic, key, payload})
		},
	})
	return svc, &msgs
}

func newOrder(symbol, trader string, price, size uint32, side secwager_pb.Side) *secwager_pb.MarketCommand {
	return &secwager_pb.MarketCommand{
		Command: &secwager_pb.MarketCommand_NewOrder{
			NewOrder: &secwager_pb.Order{
				Symbol: symbol,
				Trader: trader,
				Price:  price,
				Size:   size,
				Side:   side,
			},
		},
	}
}

func cancelCmd(symbol string, orderID uint32) *secwager_pb.MarketCommand {
	return &secwager_pb.MarketCommand{
		Command: &secwager_pb.MarketCommand_Cancel{
			Cancel: &secwager_pb.CancelRequest{
				Symbol:  symbol,
				OrderId: orderID,
			},
		},
	}
}

func execReports(msgs []published) []*secwager_pb.ExecutionReport {
	var out []*secwager_pb.ExecutionReport
	for _, m := range msgs {
		if m.topic != "order_executions" {
			continue
		}
		var r secwager_pb.ExecutionReport
		if err := proto.Unmarshal(m.payload, &r); err != nil {
			continue
		}
		out = append(out, &r)
	}
	return out
}

func depthUpdates(msgs []published) []*secwager_pb.ExecutionReport {
	// placeholder — depth updates parsed separately if needed
	return nil
}

// --- tests ---

func TestProcess_NewOrder_Accepted(t *testing.T) {
	svc, msgs := newTestService()
	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 10, secwager_pb.Side_BUY))

	reports := execReports(*msgs)
	if len(reports) != 1 {
		t.Fatalf("expected 1 ExecutionReport, got %d", len(reports))
	}
	r := reports[0]
	if r.ExecType != secwager_pb.ExecType_ORDER_ACCEPTED {
		t.Fatalf("expected ORDER_ACCEPTED, got %v", r.ExecType)
	}
	if r.Trader != "alice" || r.QtyRemaining != 10 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestProcess_FullCross_TwoReports(t *testing.T) {
	svc, msgs := newTestService()

	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 5, secwager_pb.Side_SELL))
	*msgs = nil

	svc.Process("AAPL", newOrder("AAPL", "bob", 50, 5, secwager_pb.Side_BUY))

	reports := execReports(*msgs)
	if len(reports) != 2 {
		t.Fatalf("expected 2 ExecutionReports (one per side), got %d", len(reports))
	}
	for _, r := range reports {
		if r.ExecType != secwager_pb.ExecType_ORDER_FILLED {
			t.Fatalf("expected ORDER_FILLED, got %v", r.ExecType)
		}
		if r.QtyFilled != 5 || r.QtyRemaining != 0 {
			t.Fatalf("unexpected report: %+v", r)
		}
		if r.FillPrice != 50 {
			t.Fatalf("expected fill_price=50, got %d", r.FillPrice)
		}
	}
	traders := map[string]bool{reports[0].Trader: true, reports[1].Trader: true}
	if !traders["alice"] || !traders["bob"] {
		t.Fatalf("expected reports for alice and bob, got %v", traders)
	}
}

func TestProcess_PartialFill(t *testing.T) {
	svc, msgs := newTestService()

	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 3, secwager_pb.Side_SELL))
	*msgs = nil

	svc.Process("AAPL", newOrder("AAPL", "bob", 50, 10, secwager_pb.Side_BUY))

	reports := execReports(*msgs)
	// alice fully filled, bob partially filled, bob ExecNew for remainder
	var traded, accepted int
	for _, r := range reports {
		switch r.ExecType {
		case secwager_pb.ExecType_ORDER_FILLED:
			traded++
		case secwager_pb.ExecType_ORDER_ACCEPTED:
			accepted++
			if r.Trader != "bob" || r.QtyRemaining != 7 {
				t.Fatalf("unexpected ORDER_ACCEPTED: %+v", r)
			}
		}
	}
	if traded != 2 {
		t.Fatalf("expected 2 ORDER_FILLED, got %d", traded)
	}
	if accepted != 1 {
		t.Fatalf("expected 1 ORDER_ACCEPTED for bob remainder, got %d", accepted)
	}
}

func TestProcess_Cancel(t *testing.T) {
	svc, msgs := newTestService()

	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 10, secwager_pb.Side_BUY))
	reports := execReports(*msgs)
	if len(reports) == 0 {
		t.Fatal("no ExecNew published")
	}
	orderID := reports[0].OrderId
	*msgs = nil

	svc.Process("AAPL", cancelCmd("AAPL", orderID))

	reports = execReports(*msgs)
	if len(reports) != 1 || reports[0].ExecType != secwager_pb.ExecType_ORDER_CANCELLED {
		t.Fatalf("expected ORDER_CANCELLED, got %+v", reports)
	}
	if reports[0].QtyRemaining != 10 {
		t.Fatalf("expected qty_remaining=10, got %d", reports[0].QtyRemaining)
	}
}

func TestProcess_Cancel_NotFound(t *testing.T) {
	svc, msgs := newTestService()
	svc.Process("AAPL", cancelCmd("AAPL", 999))

	reports := execReports(*msgs)
	if len(reports) != 1 || reports[0].ExecType != secwager_pb.ExecType_ORDER_REJECTED {
		t.Fatalf("expected ORDER_REJECTED for unknown order, got %+v", reports)
	}
	if reports[0].RejectReason != secwager_pb.RejectReason_ORDER_NOT_FOUND {
		t.Fatalf("expected ORDER_NOT_FOUND, got %v", reports[0].RejectReason)
	}
}

func TestProcess_SymbolIsolation(t *testing.T) {
	svc, msgs := newTestService()

	// Place a sell on AAPL and a buy on MSFT at the same price — should not cross.
	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 5, secwager_pb.Side_SELL))
	svc.Process("MSFT", newOrder("MSFT", "bob", 50, 5, secwager_pb.Side_BUY))

	reports := execReports(*msgs)
	for _, r := range reports {
		if r.ExecType == secwager_pb.ExecType_ORDER_FILLED {
			t.Fatalf("cross-symbol match occurred: %+v", r)
		}
	}
}

func TestProcess_MatchedAgainst(t *testing.T) {
	svc, msgs := newTestService()

	svc.Process("AAPL", newOrder("AAPL", "alice", 50, 5, secwager_pb.Side_SELL))
	*msgs = nil
	svc.Process("AAPL", newOrder("AAPL", "bob", 50, 5, secwager_pb.Side_BUY))

	reports := execReports(*msgs)
	for _, r := range reports {
		switch r.Trader {
		case "bob":
			if len(r.MatchedAgainst) == 0 || r.MatchedAgainst[0] != "alice" {
				t.Fatalf("bob's matched_against should be [alice], got %v", r.MatchedAgainst)
			}
		case "alice":
			if len(r.MatchedAgainst) == 0 || r.MatchedAgainst[0] != "bob" {
				t.Fatalf("alice's matched_against should be [bob], got %v", r.MatchedAgainst)
			}
		}
	}
}
