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
