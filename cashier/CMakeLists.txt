cmake_minimum_required(VERSION 3.25)
project(cashier CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(protobuf    CONFIG REQUIRED)
find_package(gRPC        CONFIG REQUIRED)
find_package(libpqxx     CONFIG REQUIRED)
find_package(etcdcpp CONFIG REQUIRED)

# ── Shared proto (messages only, no gRPC plugin) ───────────────────────────
if(NOT TARGET secwager_proto)
    add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../proto
                     ${CMAKE_CURRENT_BINARY_DIR}/proto)
endif()

# ── Cashier gRPC stubs (generated here because grpc is a cashier-only dep) ─
add_library(cashier_grpc STATIC)

get_target_property(_grpc_cpp_plugin gRPC::grpc_cpp_plugin LOCATION)

protobuf_generate(
    TARGET         cashier_grpc
    PROTOS         ${CMAKE_CURRENT_SOURCE_DIR}/../proto/cashier/cashier.proto
    LANGUAGE       grpc
    IMPORT_DIRS    ${CMAKE_CURRENT_SOURCE_DIR}/../proto
    PROTOC_OUT_DIR ${CMAKE_CURRENT_BINARY_DIR}/cashier
    PLUGIN         "protoc-gen-grpc=${_grpc_cpp_plugin}"
    GENERATE_EXTENSIONS .grpc.pb.h .grpc.pb.cc
)
protobuf_generate(
    TARGET         cashier_grpc
    PROTOS         ${CMAKE_CURRENT_SOURCE_DIR}/../proto/cashier/cashier.proto
    LANGUAGE       cpp
    IMPORT_DIRS    ${CMAKE_CURRENT_SOURCE_DIR}/../proto
    PROTOC_OUT_DIR ${CMAKE_CURRENT_BINARY_DIR}/cashier
)

target_include_directories(cashier_grpc PUBLIC ${CMAKE_CURRENT_BINARY_DIR}/cashier)
target_link_libraries(cashier_grpc
    PUBLIC secwager_proto gRPC::grpc++ protobuf::libprotobuf
)

# ── Cashier library ────────────────────────────────────────────────────────
add_library(cashier_lib STATIC
    src/postgres_cashier.cpp
    src/etcd_lock.cpp
)
target_include_directories(cashier_lib
    PUBLIC  include
    PRIVATE src ${CMAKE_CURRENT_BINARY_DIR}/cashier
)
target_link_libraries(cashier_lib
    PUBLIC  cashier_grpc
    PRIVATE libpqxx::pqxx etcdcpp
)


# ── Service binary ─────────────────────────────────────────────────────────
add_executable(cashier_server src/main.cpp)
target_include_directories(cashier_server PRIVATE src ${CMAKE_CURRENT_BINARY_DIR}/cashier)
target_link_libraries(cashier_server PRIVATE cashier_lib)

# ── Tests ──────────────────────────────────────────────────────────────────
option(BUILD_CASHIER_TESTS "Build cashier tests" ON)
if(BUILD_CASHIER_TESTS)
    enable_testing()
    find_package(GTest CONFIG REQUIRED)

    # Unit tests — in-memory FakeCashier, no external dependencies required.
    add_executable(cashier_test tests/cashier_test.cpp)
    target_include_directories(cashier_test PRIVATE include src)
    target_link_libraries(cashier_test
        PRIVATE cashier_lib GTest::gtest_main GTest::gmock
    )

    # Integration tests — self-contained: starts its own Postgres + etcd via Docker CLI.
    add_executable(cashier_integration_test tests/integration_test.cpp)
    target_include_directories(cashier_integration_test PRIVATE include src)
    target_link_libraries(cashier_integration_test
        PRIVATE cashier_lib GTest::gtest_main
    )
    include(GoogleTest)
    gtest_discover_tests(cashier_test)
    # Integration tests are NOT auto-discovered by ctest; run explicitly in CI/compose.
endif()
