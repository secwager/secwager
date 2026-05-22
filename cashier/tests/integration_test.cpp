#include "icashier.hpp"
#include "etcd_lock.hpp"
#include "postgres_cashier.hpp"
#include "cashier_service.hpp"
#include <cashier/cashier.grpc.pb.h>
#include <grpcpp/grpcpp.h>
#include <gtest/gtest.h>
#include <pqxx/pqxx>
#include <cstdlib>
#include <memory>
#include <stdexcept>
#include <string>

// ── Helpers ────────────────────────────────────────────────────────────────

static std::string require_env(const char* name) {
    const char* v = std::getenv(name);
    if (!v) throw std::runtime_error(std::string(name) + " is not set");
    return v;
}

static void truncate_tables(const std::string& dsn) {
    pqxx::connection conn{dsn};
    pqxx::work txn{conn};
    txn.exec("TRUNCATE TABLE escrow_entries, accounts RESTART IDENTITY CASCADE");
    txn.commit();
}

// ── Fixture ────────────────────────────────────────────────────────────────

class PostgresCashierIntegration : public ::testing::Test {
protected:
    static std::string pg_dsn_;
    static std::string etcd_endpoints_;

    static void SetUpTestSuite() {
        pg_dsn_         = require_env("CASHIER_PG_DSN");
        etcd_endpoints_ = require_env("CASHIER_ETCD_ENDPOINTS");
    }

    void SetUp() override {
        truncate_tables(pg_dsn_);
        auto locks = make_etcd_lock_factory(etcd_endpoints_);
        cashier_   = std::make_unique<PostgresCashier>(pg_dsn_, std::move(locks), /*pool_size=*/2);
    }

    std::unique_ptr<ICashier> cashier_;
};

std::string PostgresCashierIntegration::pg_dsn_;
std::string PostgresCashierIntegration::etcd_endpoints_;

// ── Tests ──────────────────────────────────────────────────────────────────

TEST_F(PostgresCashierIntegration, DepositCreatesAccount) {
    auto r = cashier_->deposit("alice", 500);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 500);
}

TEST_F(PostgresCashierIntegration, DepositAccumulates) {
    cashier_->deposit("alice", 300);
    auto r = cashier_->deposit("alice", 200);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.available, 500);
}

TEST_F(PostgresCashierIntegration, WithdrawSufficientFunds) {
    cashier_->deposit("alice", 500);
    auto r = cashier_->withdraw("alice", 200);
    EXPECT_EQ(r.balance,   300);
    EXPECT_EQ(r.available, 300);
}

TEST_F(PostgresCashierIntegration, WithdrawInsufficientFundsThrows) {
    cashier_->deposit("alice", 100);
    EXPECT_THROW(cashier_->withdraw("alice", 200), InsufficientFundsError);
    auto r = cashier_->check_available("alice");
    EXPECT_EQ(r.balance, 100);
}

TEST_F(PostgresCashierIntegration, EscrowLocksFunds) {
    cashier_->deposit("alice", 500);
    auto r = cashier_->escrow("alice", 42, 200);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.locked,    200);
    EXPECT_EQ(r.available, 300);
}

TEST_F(PostgresCashierIntegration, EscrowPreventsOverdraw) {
    cashier_->deposit("alice", 500);
    cashier_->escrow("alice", 1, 400);
    EXPECT_THROW(cashier_->escrow("alice", 2, 200), InsufficientFundsError);
}

TEST_F(PostgresCashierIntegration, WithdrawRespectsLockedFunds) {
    cashier_->deposit("alice", 500);
    cashier_->escrow("alice", 1, 300);
    EXPECT_THROW(cashier_->withdraw("alice", 300), InsufficientFundsError);
    auto r = cashier_->withdraw("alice", 200);
    EXPECT_EQ(r.balance,   300);
    EXPECT_EQ(r.locked,    300);
    EXPECT_EQ(r.available, 0);
}

TEST_F(PostgresCashierIntegration, ReleaseEscrowRestoresFunds) {
    cashier_->deposit("alice", 500);
    cashier_->escrow("alice", 42, 200);
    auto r = cashier_->release_escrow(42);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 500);
}

TEST_F(PostgresCashierIntegration, ReleaseUnknownEscrowThrows) {
    EXPECT_THROW(cashier_->release_escrow(999), UnknownEscrowError);
}

TEST_F(PostgresCashierIntegration, CheckAvailableUnknownTrader) {
    auto r = cashier_->check_available("ghost");
    EXPECT_EQ(r.balance,   0);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 0);
}

TEST_F(PostgresCashierIntegration, MultipleTraderIsolation) {
    cashier_->deposit("alice", 500);
    cashier_->deposit("bob",   300);
    cashier_->escrow("alice", 1, 200);
    EXPECT_EQ(cashier_->check_available("alice").available, 300);
    EXPECT_EQ(cashier_->check_available("bob").available,   300);
}

TEST_F(PostgresCashierIntegration, MultipleEscrowsSameTrader) {
    cashier_->deposit("alice", 500);
    cashier_->escrow("alice", 1, 100);
    cashier_->escrow("alice", 2, 150);
    auto r = cashier_->check_available("alice");
    EXPECT_EQ(r.locked,    250);
    EXPECT_EQ(r.available, 250);
}

// ── gRPC handler tests ─────────────────────────────────────────────────────
// Each test spins up a fresh in-process gRPC server backed by a real PostgresCashier.

class GrpcHandlerIntegration : public PostgresCashierIntegration {
protected:
    void SetUp() override {
        truncate_tables(pg_dsn_);
        auto locks   = make_etcd_lock_factory(etcd_endpoints_);
        auto cashier = std::make_shared<PostgresCashier>(pg_dsn_, std::move(locks), 2);
        service_     = std::make_unique<CashierServiceImpl>(cashier);

        int port = 0;
        grpc::ServerBuilder builder;
        builder.AddListeningPort("localhost:0", grpc::InsecureServerCredentials(), &port);
        builder.RegisterService(service_.get());
        server_ = builder.BuildAndStart();

        stub_ = cashier::CashierService::NewStub(
            grpc::CreateChannel("localhost:" + std::to_string(port),
                                grpc::InsecureChannelCredentials()));
    }

    void TearDown() override { server_->Shutdown(); }

    std::unique_ptr<CashierServiceImpl>            service_;
    std::unique_ptr<grpc::Server>                  server_;
    std::unique_ptr<cashier::CashierService::Stub> stub_;
};

TEST_F(GrpcHandlerIntegration, DepositPopulatesResponse) {
    grpc::ClientContext ctx;
    cashier::DepositRequest req;
    req.set_trader("alice"); req.set_amount(500);
    cashier::CashierResponse resp;
    auto s = stub_->Deposit(&ctx, req, &resp);
    ASSERT_TRUE(s.ok());
    EXPECT_EQ(resp.balance(),   500);
    EXPECT_EQ(resp.locked(),    0);
    EXPECT_EQ(resp.available(), 500);
}

TEST_F(GrpcHandlerIntegration, WithdrawInsufficientFunds_FailedPrecondition) {
    { grpc::ClientContext ctx; cashier::DepositRequest req; cashier::CashierResponse r;
      req.set_trader("alice"); req.set_amount(100); stub_->Deposit(&ctx, req, &r); }

    grpc::ClientContext ctx;
    cashier::WithdrawRequest req;
    req.set_trader("alice"); req.set_amount(200);
    cashier::CashierResponse resp;
    EXPECT_EQ(stub_->Withdraw(&ctx, req, &resp).error_code(),
              grpc::StatusCode::FAILED_PRECONDITION);
}

TEST_F(GrpcHandlerIntegration, WithdrawUnknownTrader_NotFound) {
    grpc::ClientContext ctx;
    cashier::WithdrawRequest req;
    req.set_trader("ghost"); req.set_amount(1);
    cashier::CashierResponse resp;
    EXPECT_EQ(stub_->Withdraw(&ctx, req, &resp).error_code(),
              grpc::StatusCode::NOT_FOUND);
}

TEST_F(GrpcHandlerIntegration, EscrowInsufficientFunds_FailedPrecondition) {
    { grpc::ClientContext ctx; cashier::DepositRequest req; cashier::CashierResponse r;
      req.set_trader("alice"); req.set_amount(100); stub_->Deposit(&ctx, req, &r); }

    grpc::ClientContext ctx;
    cashier::EscrowRequest req;
    req.set_trader("alice"); req.set_order_id(1); req.set_amount(200);
    cashier::CashierResponse resp;
    EXPECT_EQ(stub_->Escrow(&ctx, req, &resp).error_code(),
              grpc::StatusCode::FAILED_PRECONDITION);
}

TEST_F(GrpcHandlerIntegration, ReleaseEscrowUnknown_NotFound) {
    grpc::ClientContext ctx;
    cashier::ReleaseEscrowRequest req;
    req.set_order_id(999);
    cashier::CashierResponse resp;
    EXPECT_EQ(stub_->ReleaseEscrow(&ctx, req, &resp).error_code(),
              grpc::StatusCode::NOT_FOUND);
}

TEST_F(GrpcHandlerIntegration, FullLifecycle) {
    { grpc::ClientContext ctx; cashier::DepositRequest req; cashier::CashierResponse r;
      req.set_trader("alice"); req.set_amount(500);
      ASSERT_TRUE(stub_->Deposit(&ctx, req, &r).ok()); }

    { grpc::ClientContext ctx; cashier::EscrowRequest req; cashier::CashierResponse r;
      req.set_trader("alice"); req.set_order_id(42); req.set_amount(200);
      auto s = stub_->Escrow(&ctx, req, &r);
      ASSERT_TRUE(s.ok());
      EXPECT_EQ(r.locked(), 200); EXPECT_EQ(r.available(), 300); }

    { grpc::ClientContext ctx; cashier::ReleaseEscrowRequest req; cashier::CashierResponse r;
      req.set_order_id(42);
      auto s = stub_->ReleaseEscrow(&ctx, req, &r);
      ASSERT_TRUE(s.ok());
      EXPECT_EQ(r.locked(), 0); EXPECT_EQ(r.available(), 500); }

    { grpc::ClientContext ctx; cashier::WithdrawRequest req; cashier::CashierResponse r;
      req.set_trader("alice"); req.set_amount(500);
      auto s = stub_->Withdraw(&ctx, req, &r);
      ASSERT_TRUE(s.ok());
      EXPECT_EQ(r.balance(), 0); EXPECT_EQ(r.available(), 0); }
}

int main(int argc, char** argv) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}
