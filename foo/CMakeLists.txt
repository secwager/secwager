cmake_minimum_required(VERSION 3.16)
project(foo VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Crow CONFIG REQUIRED)
add_executable(foo src/main.cpp src/calculator.cpp)
target_include_directories(foo PRIVATE include)





