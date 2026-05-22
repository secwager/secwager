cmake_minimum_required(VERSION 3.30)
project(market CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(protobuf CONFIG REQUIRED)
find_package(RdKafka  CONFIG REQUIRED)

# Proto first — matchengine's CMakeLists guards against re-adding it.
add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../proto
                 ${CMAKE_CURRENT_BINARY_DIR}/proto)
add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../matchengine
                 ${CMAKE_CURRENT_BINARY_DIR}/matchengine)

# Market-specific proto (imports secwager/common.proto)
add_library(market_proto STATIC)
protobuf_generate(
    TARGET         market_proto
    PROTOS         proto/market.proto
    LANGUAGE       cpp
    IMPORT_DIRS    ${CMAKE_CURRENT_SOURCE_DIR}/proto
                   ${CMAKE_CURRENT_SOURCE_DIR}/../proto
    PROTOC_OUT_DIR ${CMAKE_CURRENT_BINARY_DIR}
)
target_include_directories(market_proto PUBLIC ${CMAKE_CURRENT_BINARY_DIR})
target_link_libraries(market_proto PUBLIC secwager_proto)

# Testable service library
add_library(market_service STATIC src/market_service.cpp)
target_include_directories(market_service
    PUBLIC  include
    PRIVATE ${CMAKE_CURRENT_BINARY_DIR}
)
target_link_libraries(market_service
    PUBLIC  matchengine market_proto secwager_proto RdKafka::rdkafka++
)

# Service binary
add_executable(market src/main.cpp)
target_link_libraries(market PRIVATE market_service)

# Tests
option(BUILD_MARKET_TESTS "Build market service tests" ON)
if(BUILD_MARKET_TESTS)
    enable_testing()
    find_package(GTest CONFIG REQUIRED)
    add_executable(market_test tests/market_service_test.cpp)
    target_include_directories(market_test
        PRIVATE ${CMAKE_CURRENT_BINARY_DIR}
    )
    target_link_libraries(market_test PRIVATE market_service GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(market_test)
endif()
