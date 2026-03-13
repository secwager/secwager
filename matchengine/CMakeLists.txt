cmake_minimum_required(VERSION 3.30)
project(matchengine CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Proto sibling — guarded so market (which adds proto first) doesn't re-add it.
if(NOT TARGET secwager_proto)
    add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../proto
                     ${CMAKE_CURRENT_BINARY_DIR}/proto)
endif()

add_library(matchengine STATIC src/engine.cpp)
target_include_directories(matchengine PUBLIC include)
target_link_libraries(matchengine PUBLIC secwager_proto)

option(BUILD_TESTING "Build tests" ON)
if(BUILD_TESTING)
    enable_testing()
    find_package(GTest CONFIG REQUIRED)
    add_executable(engine_test tests/engine_test.cpp)
    target_link_libraries(engine_test PRIVATE matchengine GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(engine_test)
endif()
