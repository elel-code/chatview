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
