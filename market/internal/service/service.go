package service

import (
	"context"
	"fmt"
	"log"
	"time"

	me "github.com/secwager/secwager/matchengine"
	market_pb "github.com/secwager/secwager/proto/gen/market"
	secwager_pb "github.com/secwager/secwager/proto/gen/secwager"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	Brokers    []string
	Topic      string // incoming_orders (consume)
	Partition  int    // from MARKET_CARDINAL
	ExecTopic  string // order_executions (publish, sync)
	DepthTopic string // depth_updates (publish, async)
	DataDir    string
	FlushEvery int           // snapshot every N messages (default 1000)
	MinBytes   int           // min fetch batch size (default 256 KB)
	MaxBytes   int           // max fetch batch size (default 10 MB)
	MaxWait    time.Duration // max wait for MinBytes (default 500ms)

	// Test hook: replaces Kafka publish when non-nil.
	Publisher func(topic, key string, payload []byte)
	// OnLive is called once when the service transitions from replay to live mode.
	OnLive func()
}

type MarketService struct {
	cfg         Config
	engines     map[string]*me.OrderBook
	offset      int64
	live        bool
	reader      *kafka.Reader
	execWriter  *kafka.Writer // sync; order_executions
	depthWriter *kafka.Writer // async; depth_updates
}

func New(cfg Config) *MarketService {
	if cfg.FlushEvery == 0 {
		cfg.FlushEvery = 1000
	}
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 256 << 10
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10 << 20
	}
	if cfg.MaxWait == 0 {
		cfg.MaxWait = 500 * time.Millisecond
	}
	s := &MarketService{
		cfg:     cfg,
		engines: make(map[string]*me.OrderBook),
	}
	if cfg.Publisher != nil {
		s.live = true
	}
	return s
}

// Run loads snapshot, replays to HWM with publishing suppressed, then enters
// the live consume loop. Blocks until ctx is cancelled, then flushes snapshot.
func (s *MarketService) Run(ctx context.Context) error {
	offset, books, err := load(s.cfg.DataDir)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	s.offset = offset

	for symbol, state := range books {
		eng := s.bookFor(symbol)
		eng.Restore(state)
	}

	s.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:   s.cfg.Brokers,
		Topic:     s.cfg.Topic,
		Partition: s.cfg.Partition,
		MinBytes:  s.cfg.MinBytes,
		MaxBytes:  s.cfg.MaxBytes,
		MaxWait:   s.cfg.MaxWait,
	})
	defer s.reader.Close()

	if err := s.reader.SetOffset(s.offset + 1); err != nil {
		return fmt.Errorf("seek reader: %w", err)
	}

	s.execWriter = &kafka.Writer{
		Addr:     kafka.TCP(s.cfg.Brokers...),
		Balancer: &kafka.LeastBytes{},
	}
	defer s.execWriter.Close()

	s.depthWriter = &kafka.Writer{
		Addr:        kafka.TCP(s.cfg.Brokers...),
		Balancer:    &kafka.LeastBytes{},
		Async:       true,
		Compression: kafka.Lz4,
	}
	defer s.depthWriter.Close()

	hwm, err := s.fetchHWM(ctx)
	if err != nil {
		return fmt.Errorf("fetch HWM: %w", err)
	}
	for s.offset+1 < hwm {
		msg, err := s.reader.FetchMessage(ctx)
		if err != nil {
			return fmt.Errorf("replay fetch: %w", err)
		}
		s.dispatchMessage(msg)
		s.offset = msg.Offset
	}

	s.live = true
	if s.cfg.OnLive != nil {
		s.cfg.OnLive()
	}

	msgCount := 0
	for {
		msg, err := s.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return persist(s.cfg.DataDir, s.offset, s.engines)
			}
			return fmt.Errorf("fetch: %w", err)
		}
		s.dispatchMessage(msg)
		s.offset = msg.Offset
		msgCount++
		if msgCount >= s.cfg.FlushEvery {
			if err := persist(s.cfg.DataDir, s.offset, s.engines); err != nil {
				log.Printf("flush snapshot: %v", err)
			}
			msgCount = 0
		}
	}
}

// Process is the test entry point — dispatches a command directly without Kafka.
func (s *MarketService) Process(symbol string, cmd *secwager_pb.MarketCommand) {
	s.processCommand(symbol, cmd)
}

func (s *MarketService) dispatchMessage(msg kafka.Message) {
	var cmd secwager_pb.MarketCommand
	if err := proto.Unmarshal(msg.Value, &cmd); err != nil {
		log.Printf("unmarshal message at offset %d: %v", msg.Offset, err)
		return
	}
	s.processCommand(string(msg.Key), &cmd)
}

func (s *MarketService) processCommand(symbol string, cmd *secwager_pb.MarketCommand) {
	book := s.bookFor(symbol)
	var events []me.Event
	switch c := cmd.Command.(type) {
	case *secwager_pb.MarketCommand_NewOrder:
		o := c.NewOrder
		var t [8]byte
		copy(t[:], o.Trader)
		side := me.Buy
		if o.Side == secwager_pb.Side_SELL {
			side = me.Sell
		}
		_, events = book.Limit(me.Order{Trader: t, Price: me.Price(o.Price), Size: me.Size(o.Size), Side: side})
	case *secwager_pb.MarketCommand_Cancel:
		events = book.Cancel(me.OrderID(c.Cancel.OrderId))
	}

	if !s.live {
		return
	}
	for _, e := range events {
		switch ev := e.(type) {
		case me.OrderEvent:
			s.publishExecReport(symbol, ev)
		case me.BBOEvent:
			s.publishDepth(symbol, ev)
		}
	}
}

func (s *MarketService) publishExecReport(symbol string, e me.OrderEvent) {
	rep := &secwager_pb.ExecutionReport{
		Symbol:       symbol,
		Trader:       e.Trader,
		OrderId:      e.OrderID,
		ExecType:     toProtoExecType(e.Type),
		QtyFilled:    e.QtyFilled,
		QtyRemaining: e.QtyRemaining,
		RejectReason: toProtoRejectReason(e.Reason),
	}
	if len(e.Fills) > 0 {
		last := e.Fills[len(e.Fills)-1]
		rep.FillPrice = uint32(last.Price)
		for _, m := range e.Fills {
			rep.MatchedAgainst = append(rep.MatchedAgainst, m.Trader)
		}
	}
	s.publishSync(s.cfg.ExecTopic, symbol, rep)
}

func (s *MarketService) publishDepth(symbol string, e me.BBOEvent) {
	s.publishAsync(s.cfg.DepthTopic, symbol, &market_pb.DepthUpdate{
		Symbol:     symbol,
		BestBid:    uint32(e.Bid),
		BestBidQty: e.BidQty,
		BestAsk:    uint32(e.Ask),
		BestAskQty: e.AskQty,
	})
}

func (s *MarketService) publishSync(topic, key string, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("marshal %T: %v", msg, err)
		return
	}
	if s.cfg.Publisher != nil {
		s.cfg.Publisher(topic, key, data)
		return
	}
	if err := s.execWriter.WriteMessages(context.Background(), kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
	}); err != nil {
		log.Printf("publish sync to %s: %v", topic, err)
	}
}

func (s *MarketService) publishAsync(topic, key string, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("marshal %T: %v", msg, err)
		return
	}
	if s.cfg.Publisher != nil {
		s.cfg.Publisher(topic, key, data)
		return
	}
	if err := s.depthWriter.WriteMessages(context.Background(), kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
	}); err != nil {
		log.Printf("publish async to %s: %v", topic, err)
	}
}

func (s *MarketService) bookFor(symbol string) *me.OrderBook {
	if eng, ok := s.engines[symbol]; ok {
		return eng
	}
	eng := me.New()
	s.engines[symbol] = eng
	return eng
}

func (s *MarketService) fetchHWM(ctx context.Context) (int64, error) {
	conn, err := kafka.DialLeader(ctx, "tcp", s.cfg.Brokers[0], s.cfg.Topic, s.cfg.Partition)
	if err != nil {
		return 0, fmt.Errorf("dial leader: %w", err)
	}
	defer conn.Close()
	_, hwm, err := conn.ReadOffsets()
	if err != nil {
		return 0, fmt.Errorf("read offsets: %w", err)
	}
	return hwm, nil
}

func toProtoExecType(t me.ExecType) secwager_pb.ExecType {
	switch t {
	case me.OrderFilled:
		return secwager_pb.ExecType_ORDER_FILLED
	case me.OrderCancelled:
		return secwager_pb.ExecType_ORDER_CANCELLED
	case me.OrderRejected:
		return secwager_pb.ExecType_ORDER_REJECTED
	default:
		return secwager_pb.ExecType_ORDER_ACCEPTED
	}
}

func toProtoRejectReason(r me.RejectReason) secwager_pb.RejectReason {
	switch r {
	case me.RejectAlreadyFilled:
		return secwager_pb.RejectReason_ORDER_ALREADY_FILLED
	case me.RejectNotFound:
		return secwager_pb.RejectReason_ORDER_NOT_FOUND
	default:
		return secwager_pb.RejectReason_REJECT_REASON_UNSPECIFIED
	}
}
