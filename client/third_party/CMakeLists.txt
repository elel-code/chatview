# ── sqlite3 native library ─────────────────────────────────────────────────────
add_subdirectory(sqlite3)

# ── sqlite_orm header-only wrapper ─────────────────────────────────────────────
add_library(sqlite_orm INTERFACE)
target_include_directories(sqlite_orm
    SYSTEM INTERFACE
    ${sqlite_orm_SOURCE_DIR}/include
)
target_link_libraries(sqlite_orm
    INTERFACE
    sqlite3
)

# ─── standalone ASIO (compiled) ────────────────────────────────────────────────
add_library(asio STATIC)
target_sources(asio
    PRIVATE
    ${asio_standalone_SOURCE_DIR}/asio/src/asio.cpp
)
target_include_directories(asio
    SYSTEM PUBLIC
    ${asio_standalone_SOURCE_DIR}/asio/include
)
target_compile_definitions(asio
    PUBLIC
    ASIO_SEPARATE_COMPILATION
)

# ── asio-grpc module wrapper ───────────────────────────────────────────────────
add_library(asio_grpc_module STATIC)
target_sources(asio_grpc_module
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    asio_grpc/asio_grpc.cppm
)
target_link_libraries(asio_grpc_module
    PUBLIC
    asio-grpc::asio-grpc-standalone-asio
    asio
    grpc++
)
