cmake_minimum_required(VERSION 3.30)
project(matchengine CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# --- Protobuf ---
find_package(protobuf CONFIG REQUIRED)

# --- Library ---
add_library(matchengine STATIC src/engine.cpp)

target_include_directories(matchengine
    PUBLIC  include
    PRIVATE ${CMAKE_CURRENT_BINARY_DIR}   # for generated book_snapshot.pb.h
)

target_link_libraries(matchengine
    PRIVATE protobuf::libprotobuf
)

# Generate C++ sources from .proto and attach to the library target
protobuf_generate(
    TARGET     matchengine
    PROTOS     proto/book_snapshot.proto
    LANGUAGE   cpp
    IMPORT_DIRS ${CMAKE_CURRENT_SOURCE_DIR}/proto
    PROTOC_OUT_DIR ${CMAKE_CURRENT_BINARY_DIR}
)

# --- Tests ---
option(BUILD_TESTING "Build tests" ON)
if(BUILD_TESTING)
    enable_testing()
    find_package(GTest CONFIG REQUIRED)

    add_executable(engine_test tests/engine_test.cpp)
    target_include_directories(engine_test
        PRIVATE include
                ${CMAKE_CURRENT_BINARY_DIR}
    )
    target_link_libraries(engine_test
        PRIVATE matchengine
                GTest::gtest_main
    )

    include(GoogleTest)
    gtest_discover_tests(engine_test)
endif()
