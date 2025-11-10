cmake_minimum_required(VERSION 3.16)


project(cashier VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)


find_package(Crow CONFIG REQUIRED)

# Cashier Service
add_executable(cashier
    src/main.cpp
    src/cashier_controller.cpp
    src/balance_service.cpp
)

target_include_directories(cashier PRIVATE include)

target_link_libraries(cashier PRIVATE
    Crow::Crow
)