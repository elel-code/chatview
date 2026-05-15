cmake_minimum_required(VERSION 4.3)
project(
    chatview
    VERSION 0.1
    LANGUAGES CXX C
)


# ── Options (must come before FetchContent) ────────────────────────────────────
include(cmake/options.cmake)

# ── Third-party dependencies ──────────────────────────────────────────────────
include(cmake/dependencies.cmake)

# ── Local third-party targets (sqlite3, asio, asio_grpc_module, …) ────────────
add_subdirectory(third_party)

# ── Project source tree ───────────────────────────────────────────────────────
add_subdirectory(src/chatview)
