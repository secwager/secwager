#pragma once
#include <memory>
#include <string>

class IEtcdLockHandle {
public:
    virtual ~IEtcdLockHandle() = default;
    IEtcdLockHandle()                                  = default;
    IEtcdLockHandle(const IEtcdLockHandle&)            = delete;
    IEtcdLockHandle& operator=(const IEtcdLockHandle&) = delete;
    IEtcdLockHandle(IEtcdLockHandle&&)                 = default;
    IEtcdLockHandle& operator=(IEtcdLockHandle&&)      = default;
};

class IEtcdLockFactory {
public:
    virtual ~IEtcdLockFactory() = default;

    [[nodiscard]]
    virtual std::unique_ptr<IEtcdLockHandle> acquire(const std::string& key) = 0;
};

std::unique_ptr<IEtcdLockFactory> make_etcd_lock_factory(const std::string& endpoints);
