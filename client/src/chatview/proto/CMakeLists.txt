# ── Protobuf/gRPC code generation ──────────────────────────────────────────────
add_executable(gRPC::grpc_cpp_plugin ALIAS grpc_cpp_plugin)

set(ASIO_GRPC_PROTO_OUT_DIR ${CMAKE_BINARY_DIR}/proto)


# Common shared types (no gRPC service)
asio_grpc_protobuf_generate(
    OUT_VAR chatview_proto_common_srcs
    OUT_DIR ${ASIO_GRPC_PROTO_OUT_DIR}
    IMPORT_DIRS ${CMAKE_SOURCE_DIR}/protobuf
    PROTOS ${CMAKE_SOURCE_DIR}/protobuf/chatview/common/types.proto
)
add_library(chatview_proto_common STATIC ${chatview_proto_common_srcs})
target_sources(chatview_proto_common
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    common.cppm
)
target_link_libraries(chatview_proto_common PUBLIC protobuf::libprotobuf)
target_include_directories(chatview_proto_common PUBLIC ${ASIO_GRPC_PROTO_OUT_DIR})


# Admin service
asio_grpc_protobuf_generate(
    GENERATE_GRPC
    OUT_VAR chatview_proto_admin_srcs
    OUT_DIR ${ASIO_GRPC_PROTO_OUT_DIR}
    IMPORT_DIRS ${CMAKE_SOURCE_DIR}/protobuf
    PROTOS ${CMAKE_SOURCE_DIR}/protobuf/chatview/admin.proto
)
add_library(chatview_proto_admin STATIC ${chatview_proto_admin_srcs})
target_sources(chatview_proto_admin
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    admin.cppm
)
target_link_libraries(chatview_proto_admin PUBLIC chatview_proto_common grpc++ protobuf::libprotobuf)
target_include_directories(chatview_proto_admin PUBLIC ${ASIO_GRPC_PROTO_OUT_DIR})


# Auth service
asio_grpc_protobuf_generate(
    GENERATE_GRPC
    OUT_VAR chatview_proto_auth_srcs
    OUT_DIR ${ASIO_GRPC_PROTO_OUT_DIR}
    IMPORT_DIRS ${CMAKE_SOURCE_DIR}/protobuf
    PROTOS ${CMAKE_SOURCE_DIR}/protobuf/chatview/auth.proto
)
add_library(chatview_proto_auth STATIC ${chatview_proto_auth_srcs})
target_sources(chatview_proto_auth
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    auth.cppm
)
target_link_libraries(chatview_proto_auth PUBLIC grpc++ protobuf::libprotobuf)
target_include_directories(chatview_proto_auth PUBLIC ${ASIO_GRPC_PROTO_OUT_DIR})


# Chat service
asio_grpc_protobuf_generate(
    GENERATE_GRPC
    OUT_VAR chatview_proto_chat_srcs
    OUT_DIR ${ASIO_GRPC_PROTO_OUT_DIR}
    IMPORT_DIRS ${CMAKE_SOURCE_DIR}/protobuf
    PROTOS ${CMAKE_SOURCE_DIR}/protobuf/chatview/chat.proto
)
add_library(chatview_proto_chat STATIC ${chatview_proto_chat_srcs})
target_sources(chatview_proto_chat
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    chat.cppm
)
target_link_libraries(chatview_proto_chat PUBLIC chatview_proto_common grpc++ protobuf::libprotobuf)
target_include_directories(chatview_proto_chat PUBLIC ${ASIO_GRPC_PROTO_OUT_DIR})


# Events service
asio_grpc_protobuf_generate(
    GENERATE_GRPC
    OUT_VAR chatview_proto_events_srcs
    OUT_DIR ${ASIO_GRPC_PROTO_OUT_DIR}
    IMPORT_DIRS ${CMAKE_SOURCE_DIR}/protobuf
    PROTOS ${CMAKE_SOURCE_DIR}/protobuf/chatview/events.proto
)
add_library(chatview_proto_events STATIC ${chatview_proto_events_srcs})
target_sources(chatview_proto_events
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    events.cppm
)
target_link_libraries(chatview_proto_events PUBLIC grpc++ protobuf::libprotobuf)
target_include_directories(chatview_proto_events PUBLIC ${ASIO_GRPC_PROTO_OUT_DIR})
