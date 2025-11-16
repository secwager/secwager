cmake_minimum_required(VERSION 3.16)
project(calculator VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Crow CONFIG REQUIRED)
add_executable(calculator src/main.cpp src/calculator.cpp src/calculator_controller.cpp)
target_include_directories(calculator PRIVATE include)
target_link_libraries(calculator PRIVATE Crow::Crow)





