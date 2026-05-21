// Copyright 2026 Dennis Hezel
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

module;

// ── Global module fragment ────────────────────────────────────────────────────
// Everything #include'd here is attached to the *global* module, not to the
// named module below. Macro definitions from these headers are visible within
// this translation unit but are NOT propagated to importers — which is exactly
// what we want: the backend macros drive compilation here and stay private.
//
// Template specialisations defined in this section (e.g.
//   std::uses_allocator<agrpc::GrpcContext, A>  in grpc_context.hpp
//   std::uses_allocator<agrpc::BasicGrpcExecutor<…>, A>  in grpc_executor.hpp
//   agrpc::asio::traits::*  specialisations in grpc_executor.hpp
// ) are reachable to importers through the exported types they are
// specialised on, per [temp.spec.partial.general]/6 and [module.reach].

#include <agrpc/alarm.hpp>
#include <agrpc/client_callback.hpp>
#include <agrpc/client_rpc.hpp>
#include <agrpc/default_server_rpc_traits.hpp>
#include <agrpc/grpc_context.hpp>
#include <agrpc/grpc_executor.hpp>
#include <agrpc/notify_on_state_change.hpp>
#include <agrpc/read.hpp>
#include <agrpc/register_awaitable_rpc_handler.hpp>
#include <agrpc/register_callback_rpc_handler.hpp>
#include <agrpc/register_coroutine_rpc_handler.hpp>
#include <agrpc/register_sender_rpc_handler.hpp>
#include <agrpc/reactor_ptr.hpp>
#include <agrpc/rpc_type.hpp>
#include <agrpc/run.hpp>
#include <agrpc/server_callback.hpp>
#include <agrpc/server_rpc.hpp>
#include <agrpc/use_sender.hpp>
#include <agrpc/waiter.hpp>

#include <chrono>
#include <cstddef>
#include <cstdint>
#include <span>
#include <string_view>

// asio types not covered by agrpc/detail/asio_forward.hpp
#include <asio/awaitable.hpp>
#include <asio/co_spawn.hpp>
#include <asio/executor_work_guard.hpp>
#include <asio/io_context.hpp>
#include <asio/ip/tcp.hpp>
#include <asio/steady_timer.hpp>
#include <asio/use_awaitable.hpp>

// gRPC types not covered by the agrpc headers' transitive includes
#include <grpcpp/channel.h>
#include <grpcpp/create_channel.h>
#include <grpcpp/security/credentials.h>
#include <grpcpp/support/channel_arguments.h>
#include <grpcpp/support/sync_stream.h>
#include <grpc/support/time.h>

// BoringSSL crypto primitives from the same build gRPC links against.
#include <openssl/aead.h>
#include <openssl/curve25519.h>
#include <openssl/evp.h>
#include <openssl/mem.h>
#include <openssl/rand.h>

// ── Named module interface ────────────────────────────────────────────────────
export module asio_grpc;

export namespace std
{
using std::coroutine_traits;
}

export namespace agrpc
{
using agrpc::Alarm;
using agrpc::allocate_reactor;
using agrpc::BasicAlarm;
using agrpc::BasicClientBidiReactor;
using agrpc::BasicClientReadReactor;
using agrpc::BasicClientUnaryReactor;
using agrpc::BasicClientWriteReactor;
using agrpc::BasicGrpcExecutor;
using agrpc::BasicServerBidiReactor;
using agrpc::BasicServerReadReactor;
using agrpc::BasicServerUnaryReactor;
using agrpc::BasicServerWriteReactor;
using agrpc::ClientBidiReactor;
using agrpc::ClientReadReactor;
using agrpc::ClientRPC;
using agrpc::ClientRPCType;
using agrpc::ClientUnaryReactor;
using agrpc::ClientWriteReactor;
using agrpc::DefaultServerRPCTraits;
using agrpc::GenericServerRPC;
using agrpc::GenericStreamingClientRPC;
using agrpc::GenericUnaryClientRPC;
using agrpc::GrpcContext;
using agrpc::GrpcExecutor;
using agrpc::make_reactor;
using agrpc::notify_on_state_change;
using agrpc::ReactorPtr;
using agrpc::read;
using agrpc::register_sender_rpc_handler;
using agrpc::ServerBidiReactor;
using agrpc::ServerReadReactor;
using agrpc::ServerRPC;
using agrpc::ServerRPCPtr;
using agrpc::ServerRPCType;
using agrpc::ServerUnaryReactor;
using agrpc::ServerWriteReactor;
using agrpc::unary_call;
using agrpc::use_sender;
using agrpc::UseSender;
using agrpc::Waiter;

#if defined(AGRPC_STANDALONE_ASIO) || defined(AGRPC_BOOST_ASIO)
using agrpc::DefaultRunTraits;
using agrpc::register_callback_rpc_handler;
using agrpc::run;
using agrpc::run_completion_queue;
#ifdef AGRPC_ASIO_HAS_CO_AWAIT
using agrpc::register_awaitable_rpc_handler;
using agrpc::register_coroutine_rpc_handler;
#endif
#endif
}

// ── Exported gRPC types (client-focused) ──────────────────────────────────────

export namespace grpc
{
using grpc::Channel;
using grpc::ChannelArguments;
using grpc::ChannelInterface;
using grpc::ClientContext;
using grpc::ClientReaderWriter;
using grpc::CreateChannel;
using grpc::CreateCustomChannel;
using grpc::InsecureChannelCredentials;
using grpc::SslCredentials;
using grpc::SslCredentialsOptions;
using grpc::Status;
using grpc::StatusCode;

auto monotonic_deadline_after(std::chrono::steady_clock::duration timeout) -> gpr_timespec
{
    const auto micros = std::chrono::duration_cast<std::chrono::microseconds>(timeout);
    return gpr_time_add(gpr_now(GPR_CLOCK_MONOTONIC), gpr_time_from_micros(micros.count(), GPR_TIMESPAN));
}
}

// ── Exported BoringSSL helpers ────────────────────────────────────────────────

export namespace bssl
{
constexpr std::size_t ed25519_seed_size = 32;
constexpr std::size_t ed25519_public_key_size = ED25519_PUBLIC_KEY_LEN;
constexpr std::size_t ed25519_private_key_size = ED25519_PRIVATE_KEY_LEN;
constexpr std::size_t ed25519_signature_size = ED25519_SIGNATURE_LEN;
constexpr std::size_t identity_salt_size = 16;

auto secure_zero(std::span<unsigned char> bytes) -> void
{
    if (!bytes.empty()) {
        OPENSSL_cleanse(bytes.data(), bytes.size());
    }
}

auto random_bytes(std::span<unsigned char> bytes) -> bool
{
    return bytes.empty() || RAND_bytes(bytes.data(), bytes.size()) == 1;
}

auto derive_scrypt_key(
    std::span<unsigned char> key,
    std::string_view password,
    std::span<const unsigned char> salt) -> bool
{
    constexpr std::uint64_t n = 1ull << 15;
    constexpr std::uint64_t r = 8;
    constexpr std::uint64_t p = 1;
    constexpr std::size_t max_mem = 64ull * 1024ull * 1024ull;
    return EVP_PBE_scrypt(
        password.data(),
        password.size(),
        salt.data(),
        salt.size(),
        n,
        r,
        p,
        max_mem,
        key.data(),
        key.size()) == 1;
}

auto ed25519_keypair_from_seed(
    std::span<unsigned char> public_key,
    std::span<unsigned char> private_key,
    std::span<const unsigned char> seed) -> bool
{
    if (public_key.size() != ed25519_public_key_size ||
        private_key.size() != ed25519_private_key_size ||
        seed.size() != ed25519_seed_size) {
        return false;
    }
    ED25519_keypair_from_seed(public_key.data(), private_key.data(), seed.data());
    return true;
}

auto ed25519_sign(
    std::span<unsigned char> signature,
    std::span<const unsigned char> message,
    std::span<const unsigned char> private_key) -> bool
{
    if (signature.size() != ed25519_signature_size || private_key.size() != ed25519_private_key_size) {
        return false;
    }
    return ED25519_sign(signature.data(), message.data(), message.size(), private_key.data()) == 1;
}

auto xchacha20_poly1305_key_size() -> std::size_t
{
    return EVP_AEAD_key_length(EVP_aead_xchacha20_poly1305());
}

auto xchacha20_poly1305_nonce_size() -> std::size_t
{
    return EVP_AEAD_nonce_length(EVP_aead_xchacha20_poly1305());
}

auto xchacha20_poly1305_overhead() -> std::size_t
{
    return EVP_AEAD_max_overhead(EVP_aead_xchacha20_poly1305());
}

auto xchacha20_poly1305_seal(
    std::span<unsigned char> output,
    std::size_t& output_size,
    std::span<const unsigned char> key,
    std::span<const unsigned char> nonce,
    std::span<const unsigned char> input) -> bool
{
    EVP_AEAD_CTX context{};
    const auto* aead = EVP_aead_xchacha20_poly1305();
    if (EVP_AEAD_CTX_init(&context, aead, key.data(), key.size(), EVP_AEAD_DEFAULT_TAG_LENGTH, nullptr) != 1) {
        output_size = 0;
        return false;
    }
    const auto ok = EVP_AEAD_CTX_seal(
        &context,
        output.data(),
        &output_size,
        output.size(),
        nonce.data(),
        nonce.size(),
        input.data(),
        input.size(),
        nullptr,
        0) == 1;
    EVP_AEAD_CTX_cleanup(&context);
    return ok;
}

auto xchacha20_poly1305_open(
    std::span<unsigned char> output,
    std::size_t& output_size,
    std::span<const unsigned char> key,
    std::span<const unsigned char> nonce,
    std::span<const unsigned char> input) -> bool
{
    EVP_AEAD_CTX context{};
    const auto* aead = EVP_aead_xchacha20_poly1305();
    if (EVP_AEAD_CTX_init(&context, aead, key.data(), key.size(), EVP_AEAD_DEFAULT_TAG_LENGTH, nullptr) != 1) {
        output_size = 0;
        return false;
    }
    const auto ok = EVP_AEAD_CTX_open(
        &context,
        output.data(),
        &output_size,
        output.size(),
        nonce.data(),
        nonce.size(),
        input.data(),
        input.size(),
        nullptr,
        0) == 1;
    EVP_AEAD_CTX_cleanup(&context);
    return ok;
}
}

// ── Exported asio types ───────────────────────────────────────────────────────

export namespace asio
{
using asio::awaitable;
using asio::co_spawn;
using asio::error_code;
using asio::executor_work_guard;
using asio::io_context;
using asio::make_work_guard;
using asio::steady_timer;
using asio::use_awaitable;

namespace ip
{
using asio::ip::tcp;
}
}
