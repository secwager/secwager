cmake_minimum_required(VERSION 3.15)
project(asio-fun)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
# Find Asio package
find_package(asio CONFIG REQUIRED)
# Add executable
add_executable(asio-fun src/main.cpp)
# Link Asio library
target_link_libraries(asio-fun PRIVATE asio::asio)
# Include directories
target_include_directories(asio-fun PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/include)

