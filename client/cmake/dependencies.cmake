# ── FetchContent declarations ──────────────────────────────────────────────────
include(FetchContent)


FetchContent_Declare(
    grpc
    GIT_TAG v1.80.0
    GIT_REPOSITORY "https://github.com/grpc/grpc.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
    GIT_SUBMODULES ""
    PATCH_COMMAND git submodule update --init --recursive --depth 1
)

FetchContent_Declare(
    asio_standalone
    GIT_TAG asio-1-38-0
    GIT_REPOSITORY "https://github.com/chriskohlhoff/asio.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
)

FetchContent_Declare(
    asio_grpc
    GIT_TAG v3.7.0
    GIT_REPOSITORY "https://github.com/Tradias/asio-grpc.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
)

FetchContent_Declare(
    glaze
    GIT_TAG v7.6.0
    GIT_REPOSITORY "https://github.com/stephenberry/glaze.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
)

FetchContent_Declare(
    saucer
    GIT_TAG v8.0.5
    GIT_REPOSITORY "https://github.com/saucer/saucer.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
)

FetchContent_Declare(
    sml
    GIT_TAG v1.1.13
    GIT_REPOSITORY "https://github.com/boost-ext/sml.git"
    EXCLUDE_FROM_ALL
    SYSTEM
    GIT_SHALLOW TRUE
)

FetchContent_Declare(
    libsodium
    GIT_REPOSITORY "https://github.com/robinlinden/libsodium-cmake.git"
    GIT_TAG cfebfd3da486d5a86c644c8b47067e5411c7599c
    EXCLUDE_FROM_ALL
    SYSTEM
)

FetchContent_Declare(
    sqlite_orm
    GIT_REPOSITORY "https://github.com/fnc12/sqlite_orm.git"
    GIT_TAG v1.9.1
    GIT_SHALLOW TRUE
    SYSTEM
    SOURCE_SUBDIR "empty_dir_to_skip_build"
)

# ── Fetch & make available ─────────────────────────────────────────────────────
FetchContent_MakeAvailable(
    asio_standalone
    grpc
    asio_grpc
    glaze
    saucer
    sml
    libsodium
    sqlite_orm
)
