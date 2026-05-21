//go:build integration

package service

import (
	"context"
	"os"
	"testing"
	"time"

	me "github.com/secwager/secwager/matchengine"
	secwager_pb "github.com/secwager/secwager/proto/gen/secwager"
	"github.com/segmentio/kafka-go"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"google.golang.org/protobuf/proto"
)

const (
	inTopic    = "incoming_orders"
	execTopic  = "order_executions"
	depthTopic = "depth_updates"
)

var sharedBroker string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := redpanda.Run(ctx, "redpandadata/redpanda:v24.1.1")
	if err != nil {
		panic("start redpanda: " + err.Error())
	}
	defer container.Terminate(ctx)

	sharedBroker, err = container.KafkaSeedBroker(ctx)
	if err != nil {
		panic("get broker: " + err.Error())
	}

	conn, err := kafka.Dial("tcp", sharedBroker)
	if err != nil {
		panic("dial kafka: " + err.Error())
	}
	err = conn.CreateTopics(
		kafka.TopicConfig{Topic: inTopic, NumPartitions: 1, ReplicationFactor: 1},
		kafka.TopicConfig{Topic: execTopic, NumPartitions: 1, ReplicationFactor: 1},
		kafka.TopicConfig{Topic: depthTopic, NumPartitions: 1, ReplicationFactor: 1},
	)
	conn.Close()
	if err != nil {
		panic("create topics: " + err.Error())
	}

	os.Exit(m.Run())
}

// primeDataDir writes an empty snapshot at the current incoming_orders HWM so the
// service starts consuming from "now" rather than replaying prior test messages.
func primeDataDir(t *testing.T, dataDir string) {
	t.Helper()
	conn, err := kafka.DialLeader(context.Background(), "tcp", sharedBroker, inTopic, 0)
	if err != nil {
		t.Fatalf("prime: dial: %v", err)
	}
	_, hwm, err := conn.ReadOffsets()
	conn.Close()
	if err != nil {
		t.Fatalf("prime: read offsets: %v", err)
	}
	// offset stored = hwm-1 so service seeks to hwm on boot, skipping all prior messages.
	if err := persist(dataDir, hwm-1, map[string]*me.OrderBook{}); err != nil {
		t.Fatalf("prime: persist: %v", err)
	}
}

// startService runs a MarketService against sharedBroker and blocks until live.
// Returns a cancel func that triggers graceful shutdown and waits for it.
func startService(t *testing.T, dataDir string) (*MarketService, func()) {
	t.Helper()
	ready := make(chan struct{})
	cfg := Config{
		Brokers:    []string{sharedBroker},
		Topic:      inTopic,
		Partition:  0,
		ExecTopic:  execTopic,
		DepthTopic: depthTopic,
		DataDir:    dataDir,
		FlushEvery: 1000,
		OnLive:     func() { close(ready) },
	}
	svc := New(cfg)
	ctx, cancelFn := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()

	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		cancelFn()
		t.Fatal("service never went live")
	}
	return svc, func() { cancelFn(); <-done }
}

// execReader opens a manual-partition reader on the execTopic starting at the
// current end offset (high watermark). Each test gets its own start point so
// messages from prior tests are not visible, and no group-join delay occurs.
func execReader(t *testing.T) *kafka.Reader {
	t.Helper()
	conn, err := kafka.DialLeader(context.Background(), "tcp", sharedBroker, execTopic, 0)
	if err != nil {
		t.Fatalf("dial exec topic: %v", err)
	}
	_, hwm, err := conn.ReadOffsets()
	conn.Close()
	if err != nil {
		t.Fatalf("read hwm: %v", err)
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{sharedBroker},
		Topic:     execTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  1 << 20,
	})
	r.SetOffset(hwm)
	return r
}

func readExecReports(t *testing.T, r *kafka.Reader, n int) []*secwager_pb.ExecutionReport {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var reports []*secwager_pb.ExecutionReport
	for len(reports) < n {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			t.Fatalf("fetch exec report (%d/%d): %v", len(reports), n, err)
		}
		var rep secwager_pb.ExecutionReport
		if err := proto.Unmarshal(msg.Value, &rep); err != nil {
			t.Fatalf("unmarshal exec report: %v", err)
		}
		reports = append(reports, &rep)
	}
	return reports
}

func publishMarketCmd(t *testing.T, symbol string, cmd *secwager_pb.MarketCommand) {
	t.Helper()
	data, err := proto.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	w := &kafka.Writer{Addr: kafka.TCP(sharedBroker), Topic: inTopic, Balancer: &kafka.LeastBytes{}}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{Key: []byte(symbol), Value: data}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// --- tests ---

func TestIntegration_NewOrder_PublishesExecNew(t *testing.T) {
	r := execReader(t)
	defer r.Close()

	dir := t.TempDir()
	primeDataDir(t, dir)
	_, cancel := startService(t, dir)
	defer cancel()

	publishMarketCmd(t, "AAPL", newOrder("AAPL", "alice", 50, 10, secwager_pb.Side_BUY))

	reports := readExecReports(t, r, 1)
	rep := reports[0]
	if rep.ExecType != secwager_pb.ExecType_ORDER_ACCEPTED {
		t.Fatalf("expected ORDER_ACCEPTED, got %v", rep.ExecType)
	}
	if rep.Trader != "alice" || rep.Symbol != "AAPL" || rep.QtyRemaining != 10 {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestIntegration_FullCross_PublishesTwoTrades(t *testing.T) {
	r := execReader(t)
	defer r.Close()

	dir := t.TempDir()
	primeDataDir(t, dir)
	_, cancel := startService(t, dir)
	defer cancel()

	publishMarketCmd(t, "AAPL", newOrder("AAPL", "alice", 50, 5, secwager_pb.Side_SELL))
	publishMarketCmd(t, "AAPL", newOrder("AAPL", "bob", 50, 5, secwager_pb.Side_BUY))

	// alice ExecNew + 2× ORDER_FILLED
	reports := readExecReports(t, r, 3)
	var newCount, tradeCount int
	for _, rep := range reports {
		switch rep.ExecType {
		case secwager_pb.ExecType_ORDER_ACCEPTED:
			newCount++
		case secwager_pb.ExecType_ORDER_FILLED:
			tradeCount++
			if rep.QtyFilled != 5 || rep.QtyRemaining != 0 || rep.FillPrice != 50 {
				t.Fatalf("trade report: %+v", rep)
			}
		}
	}
	if newCount != 1 || tradeCount != 2 {
		t.Fatalf("expected 1 ORDER_ACCEPTED + 2 ORDER_FILLED, got %d + %d", newCount, tradeCount)
	}
}

func TestIntegration_Cancel_PublishesCancelled(t *testing.T) {
	r := execReader(t)
	defer r.Close()

	dir := t.TempDir()
	primeDataDir(t, dir)
	_, cancel := startService(t, dir)
	defer cancel()

	publishMarketCmd(t, "AAPL", newOrder("AAPL", "alice", 50, 10, secwager_pb.Side_BUY))
	reports := readExecReports(t, r, 1)
	orderID := reports[0].OrderId

	publishMarketCmd(t, "AAPL", cancelCmd("AAPL", orderID))
	reports = readExecReports(t, r, 1)

	rep := reports[0]
	if rep.ExecType != secwager_pb.ExecType_ORDER_CANCELLED {
		t.Fatalf("expected ORDER_CANCELLED, got %v", rep.ExecType)
	}
	if rep.QtyRemaining != 10 {
		t.Fatalf("expected qty_remaining=10, got %d", rep.QtyRemaining)
	}
}

func TestIntegration_Restart_RestoresFromSnapshot(t *testing.T) {
	dataDir := t.TempDir()
	primeDataDir(t, dataDir) // skip messages from prior tests

	// First run: place a resting buy, flush every message, then shut down.
	r1 := execReader(t)
	ready1 := make(chan struct{})
	cfg1 := Config{
		Brokers: []string{sharedBroker}, Topic: inTopic, Partition: 0,
		ExecTopic: execTopic, DepthTopic: depthTopic,
		DataDir: dataDir, FlushEvery: 1,
		OnLive: func() { close(ready1) },
	}
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan error, 1)
	go func() { done1 <- New(cfg1).Run(ctx1) }()
	select {
	case <-ready1:
	case <-time.After(30 * time.Second):
		cancel1()
		t.Fatal("first service never went live")
	}

	publishMarketCmd(t, "AAPL", newOrder("AAPL", "alice", 50, 10, secwager_pb.Side_BUY))
	readExecReports(t, r1, 1) // wait for message processed + snapshot flushed
	r1.Close()
	cancel1()
	<-done1

	// Second run: restore from the snapshot written by the first service.
	// Alice's resting buy should survive the restart.
	r2 := execReader(t)
	defer r2.Close()

	_, cancel2 := startService(t, dataDir) // dataDir has first service's snapshot
	defer cancel2()

	// Crossing sell should match alice's restored buy → 2 ORDER_FILLED.
	publishMarketCmd(t, "AAPL", newOrder("AAPL", "bob", 50, 10, secwager_pb.Side_SELL))
	reports := readExecReports(t, r2, 2)
	for _, rep := range reports {
		if rep.ExecType != secwager_pb.ExecType_ORDER_FILLED {
			t.Fatalf("expected ORDER_FILLED after restart, got %v (%+v)", rep.ExecType, rep)
		}
	}
}
