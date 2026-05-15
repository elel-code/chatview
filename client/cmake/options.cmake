# ── Compiler & language options ────────────────────────────────────────────────
set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_WARN_DEPRECATED OFF CACHE BOOL "Suppress third-party CMake deprecation warnings" FORCE)
set(BUILD_TESTING OFF CACHE BOOL "" FORCE)
option(CHATVIEW_BUILD_TESTS "Build ChatView client tests" ON)
set(BUILD_EXAMPLES OFF CACHE BOOL "Disable examples" FORCE)


# ── Third-party build options ──────────────────────────────────────────────────

# Disable all non-C++ gRPC plugins
set(gRPC_BUILD_TESTS OFF CACHE BOOL "Disable gRPC tests" FORCE)
set(gRPC_BUILD_CSHARP_EXT OFF CACHE BOOL "Disable C# ext" FORCE)
set(gRPC_BUILD_GRPC_CSHARP_PLUGIN OFF CACHE BOOL "Disable C# plugin" FORCE)
set(gRPC_BUILD_GRPC_NODE_PLUGIN OFF CACHE BOOL "Disable Node plugin" FORCE)
set(gRPC_BUILD_GRPC_OBJECTIVE_C_PLUGIN OFF CACHE BOOL "Disable ObjC plugin" FORCE)
set(gRPC_BUILD_GRPC_PHP_PLUGIN OFF CACHE BOOL "Disable PHP plugin" FORCE)
set(gRPC_BUILD_GRPC_PYTHON_PLUGIN OFF CACHE BOOL "Disable Python plugin" FORCE)
set(gRPC_BUILD_GRPC_RUBY_PLUGIN OFF CACHE BOOL "Disable Ruby plugin" FORCE)
set(gRPC_BUILD_GRPC_CPP_PLUGIN ON CACHE BOOL "Enable C++ plugin" FORCE)

# Optimise Protobuf
set(protobuf_BUILD_TESTS OFF CACHE BOOL "Disable protobuf tests" FORCE)
set(protobuf_BUILD_EXAMPLES OFF CACHE BOOL "Disable protobuf examples" FORCE)
set(protobuf_INSTALL OFF CACHE BOOL "Disable protobuf install" FORCE)

# Optimise Abseil & RE2
set(ABSL_BUILD_TESTING OFF CACHE BOOL "Disable Abseil tests" FORCE)
set(RE2_BUILD_TESTING OFF CACHE BOOL "Disable RE2 tests" FORCE)

# libsodium
set(SODIUM_DISABLE_TESTS ON CACHE BOOL "Disable libsodium tests" FORCE)
