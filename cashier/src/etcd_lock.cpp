#include "etcd_lock.hpp"
#include <libetcd/rpc.grpc.pb.h>
#include <grpcpp/grpcpp.h>
#include <stdexcept>
#include <thread>
#include <chrono>

namespace {

class EtcdLockHandle final : public IEtcdLockHandle {
public:
    EtcdLockHandle(std::shared_ptr<grpc::Channel> channel, int64_t lease_id, std::string key)
        : kv_(etcdserverpb::KV::NewStub(channel))
        , lease_(etcdserverpb::Lease::NewStub(channel))
        , lease_id_(lease_id)
        , key_(std::move(key)) {}

    ~EtcdLockHandle() override {
        try {
            grpc::ClientContext ctx1;
            etcdserverpb::DeleteRangeRequest del;
            del.set_key(key_);
            etcdserverpb::DeleteRangeResponse del_resp;
            kv_->DeleteRange(&ctx1, del, &del_resp);

            grpc::ClientContext ctx2;
            etcdserverpb::LeaseRevokeRequest rev;
            rev.set_id(lease_id_);
            etcdserverpb::LeaseRevokeResponse rev_resp;
            lease_->LeaseRevoke(&ctx2, rev, &rev_resp);
        } catch (...) {}
    }

private:
    std::unique_ptr<etcdserverpb::KV::Stub>    kv_;
    std::unique_ptr<etcdserverpb::Lease::Stub> lease_;
    int64_t     lease_id_;
    std::string key_;
};

} // namespace

class EtcdLockFactory final : public IEtcdLockFactory {
public:
    explicit EtcdLockFactory(const std::string& endpoints) {
        std::string target = endpoints;
        if (target.rfind("http://",  0) == 0) target = target.substr(7);
        if (target.rfind("https://", 0) == 0) target = target.substr(8);
        channel_ = grpc::CreateChannel(target, grpc::InsecureChannelCredentials());
    }

    std::unique_ptr<IEtcdLockHandle> acquire(const std::string& key) override {
        auto lease = etcdserverpb::Lease::NewStub(channel_);
        auto kv    = etcdserverpb::KV::NewStub(channel_);

        // Grant a lease so the lock self-expires if the process dies.
        grpc::ClientContext lease_ctx;
        etcdserverpb::LeaseGrantRequest grant_req;
        grant_req.set_ttl(30);
        etcdserverpb::LeaseGrantResponse grant_resp;
        auto status = lease->LeaseGrant(&lease_ctx, grant_req, &grant_resp);
        if (!status.ok())
            throw std::runtime_error("etcd LeaseGrant failed for " + key + ": " + status.error_message());
        int64_t lease_id = grant_resp.id();

        // Spin: put the key (with the lease) only if it does not already exist.
        for (int i = 0; i < 200; ++i) {
            grpc::ClientContext txn_ctx;
            etcdserverpb::TxnRequest txn;

            auto* cmp = txn.add_compare();
            cmp->set_key(key);
            cmp->set_target(etcdserverpb::Compare_CompareTarget_VERSION);
            cmp->set_result(etcdserverpb::Compare_CompareResult_EQUAL);
            cmp->set_version(0);

            auto* put = txn.add_success()->mutable_request_put();
            put->set_key(key);
            put->set_value("");
            put->set_lease(lease_id);

            etcdserverpb::TxnResponse txn_resp;
            status = kv->Txn(&txn_ctx, txn, &txn_resp);
            if (!status.ok()) {
                revoke(lease_id);
                throw std::runtime_error("etcd Txn failed for " + key + ": " + status.error_message());
            }
            if (txn_resp.succeeded())
                return std::make_unique<EtcdLockHandle>(channel_, lease_id, key);

            std::this_thread::sleep_for(std::chrono::milliseconds(50));
        }

        revoke(lease_id);
        throw std::runtime_error("etcd lock timeout for key " + key);
    }

private:
    void revoke(int64_t lease_id) {
        auto lease = etcdserverpb::Lease::NewStub(channel_);
        grpc::ClientContext ctx;
        etcdserverpb::LeaseRevokeRequest req;
        req.set_id(lease_id);
        etcdserverpb::LeaseRevokeResponse resp;
        lease->LeaseRevoke(&ctx, req, &resp);
    }

    std::shared_ptr<grpc::Channel> channel_;
};

std::unique_ptr<IEtcdLockFactory> make_etcd_lock_factory(const std::string& endpoints) {
    return std::make_unique<EtcdLockFactory>(endpoints);
}
