#pragma once
#include "icashier.hpp"
#include <cashier/cashier.grpc.pb.h>
#include <grpcpp/grpcpp.h>
#include <functional>
#include <memory>

class CashierServiceImpl final : public cashier::CashierService::Service {
public:
    explicit CashierServiceImpl(std::shared_ptr<ICashier> cashier)
        : cashier_(std::move(cashier)) {}

    grpc::Status Deposit(grpc::ServerContext*,
                         const cashier::DepositRequest* req,
                         cashier::CashierResponse*      resp) override {
        return call([&] { return cashier_->deposit(req->trader(), req->amount()); }, resp);
    }

    grpc::Status Withdraw(grpc::ServerContext*,
                          const cashier::WithdrawRequest* req,
                          cashier::CashierResponse*       resp) override {
        return call([&] { return cashier_->withdraw(req->trader(), req->amount()); }, resp);
    }

    grpc::Status Escrow(grpc::ServerContext*,
                        const cashier::EscrowRequest* req,
                        cashier::CashierResponse*     resp) override {
        return call([&] {
            return cashier_->escrow(req->trader(), req->order_id(), req->amount());
        }, resp);
    }

    grpc::Status ReleaseEscrow(grpc::ServerContext*,
                               const cashier::ReleaseEscrowRequest* req,
                               cashier::CashierResponse*            resp) override {
        return call([&] { return cashier_->release_escrow(req->order_id()); }, resp);
    }

    grpc::Status CheckAvailable(grpc::ServerContext*,
                                const cashier::CheckRequest* req,
                                cashier::CashierResponse*    resp) override {
        return call([&] { return cashier_->check_available(req->trader()); }, resp);
    }

private:
    static grpc::Status call(std::function<CashierResult()> fn,
                             cashier::CashierResponse*      resp) {
        try {
            auto r = fn();
            resp->set_balance(r.balance);
            resp->set_locked(r.locked);
            resp->set_available(r.available);
            return grpc::Status::OK;
        } catch (const InsufficientFundsError& e) {
            return {grpc::StatusCode::FAILED_PRECONDITION, e.what()};
        } catch (const UnknownTraderError& e) {
            return {grpc::StatusCode::NOT_FOUND, e.what()};
        } catch (const UnknownEscrowError& e) {
            return {grpc::StatusCode::NOT_FOUND, e.what()};
        } catch (const std::invalid_argument& e) {
            return {grpc::StatusCode::INVALID_ARGUMENT, e.what()};
        } catch (const std::exception& e) {
            return {grpc::StatusCode::INTERNAL, e.what()};
        }
    }

    std::shared_ptr<ICashier> cashier_;
};
