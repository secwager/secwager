cmake_minimum_required(VERSION 3.30)
project(calculator-client)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Find dependencies
find_package(cpr CONFIG REQUIRED)
find_package(nlohmann_json CONFIG REQUIRED)
find_package(fmt CONFIG REQUIRED)

# Add static library
add_library(calculator-client STATIC
    src/calculator_client.cpp
    src/cpr_http_client.cpp
)

# Include directories - PUBLIC so tests can use them
target_include_directories(calculator-client PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/include
)

# Link libraries
target_link_libraries(calculator-client PRIVATE
    cpr::cpr
    nlohmann_json::nlohmann_json
    fmt::fmt
)

# Testing
option(BUILD_TESTING "Build tests" ON)
if(BUILD_TESTING)
    enable_testing()
    find_package(GTest CONFIG REQUIRED)
    
    add_executable(calculator-client-tests
        tests/calculator_client_test.cpp
    )
    
    target_link_libraries(calculator-client-tests PRIVATE
        calculator-client
        GTest::gtest
        GTest::gtest_main
    )
    
    include(GoogleTest)
    gtest_discover_tests(calculator-client-tests)
endif()