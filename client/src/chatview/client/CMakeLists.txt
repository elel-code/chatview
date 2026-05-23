# ── ChatView client core library ───────────────────────────────────────────────
add_library(chatview_client_core STATIC)
target_sources(chatview_client_core
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    client.cppm
    client-types.cppm
    client-detail.cppm
    client-identity.cppm
    client-rpc.cppm
    client-bridge.cppm
    client-session.cppm
    client-cache.cppm
    client-outbox.cppm
    client-native.cppm
)
target_sources(chatview_client_core
    PRIVATE
    client-rpc.cpp
)
add_dependencies(chatview_client_core chatview_proto_generated)
target_link_libraries(chatview_client_core
    PUBLIC
    asio_grpc_module
    chatview_proto_admin
    chatview_proto_auth
    chatview_proto_chat
    chatview_proto_events
    chatview_storage_cache
    cr::coco
    sqlite_orm
    grpc++
)

if(WIN32 AND CMAKE_GENERATOR MATCHES "Ninja" AND CMAKE_CXX_COMPILER_ID MATCHES "Clang")
    set_property(GLOBAL APPEND PROPERTY JOB_POOLS chatview_client_modules=1)
    set_property(TARGET chatview_client_core PROPERTY JOB_POOL_COMPILE chatview_client_modules)
endif()
