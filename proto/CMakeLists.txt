cmake_minimum_required(VERSION 3.30)
project(secwager_proto CXX)
set(CMAKE_CXX_STANDARD 20)

# Guard: safe to add_subdirectory this from multiple sibling components.
if(TARGET secwager_proto)
    return()
endif()

find_package(protobuf CONFIG REQUIRED)

add_library(secwager_proto STATIC)
protobuf_generate(
    TARGET         secwager_proto
    PROTOS         secwager/common.proto
                   secwager/book_snapshot.proto
    LANGUAGE       cpp
    IMPORT_DIRS    ${CMAKE_CURRENT_SOURCE_DIR}
    PROTOC_OUT_DIR ${CMAKE_CURRENT_BINARY_DIR}
)
target_include_directories(secwager_proto PUBLIC ${CMAKE_CURRENT_BINARY_DIR})
target_link_libraries(secwager_proto PUBLIC protobuf::libprotobuf)
