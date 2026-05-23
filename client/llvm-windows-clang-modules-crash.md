# LLVM Windows Clang Modules Crash Notes

This document records the Windows-only Clang crash seen while building the ChatView C++ client, for later reduction and filing upstream to LLVM.

## Environment

- OS: GitHub Actions `windows-latest`
- Compiler: LLVM/Clang `22.1.6`
- Target: `x86_64-pc-windows-msvc`
- Generator: Ninja
- CMake: `>=4.3,<4.4`
- Language mode: C++23
- Standard library / SDK: Visual Studio 18 / MSVC 14.51 / Windows SDK 10.0.26100.0 from the runner
- Relevant libraries:
  - `asio-grpc` 3.7.0
  - standalone Asio 1.38.0
  - gRPC 1.80.0

The build uses `clang++.exe`, `clang-scan-deps.exe`, and `lld-link.exe` from `C:\LLVM\bin`, with the Visual Studio developer environment loaded only for MSVC/Windows SDK paths.

## Symptom

`client-rpc.cppm` compiles, then Clang crashes while compiling an importing module such as `client-bridge.cppm`, `client-session.cppm`, or `client-cache.cppm`.

Representative failure:

```text
FAILED: src/chatview/client/CMakeFiles/chatview_client_core.dir/client-bridge.cppm.obj
C:\LLVM\bin\clang++.exe ...
PLEASE submit a bug report to https://github.com/llvm/llvm-project/issues/
Exception Code: 0xC0000005
```

Earlier crashes occurred while mangling helper templates in `client-rpc.cppm`:

```text
LLVM IR generation of declaration 'chatview::client::RpcClient::unary_once'
Mangling declaration 'chatview::client::RpcClient::unary_once'
```

After moving those helpers out of the class:

```text
LLVM IR generation of declaration 'chatview::client::rpc_detail::unary_once'
Mangling declaration 'chatview::client::rpc_detail::unary_once'
```

After removing the helper templates from the RPC module interface, the crash moved into Asio:

```text
asio/impl/use_awaitable.hpp:213:22:
LLVM IR generation of declaration
'asio::async_result<asio::use_awaitable_t<>, void (grpc::Status)>::initiate'
Mangling declaration
'asio::async_result<asio::use_awaitable_t<>, void (grpc::Status)>::initiate'
```

A later run hit the same Asio-shaped crash while compiling `client-session.cppm`:

```text
client-session.cppm:154:41: current parser token ';'
instantiating class definition 'std::optional<chatview::client::SessionController::Lockout>'
asio/impl/use_awaitable.hpp:213:22:
LLVM IR generation of declaration
'asio::async_result<asio::use_awaitable_t<>, void (grpc::Status)>::initiate'
Mangling declaration
'asio::async_result<asio::use_awaitable_t<>, void (grpc::Status)>::initiate'
Exception Code: 0xC0000005
```

After removing the direct RPC dependency from `client-session.cppm`, a later run moved the crash to `client-cache.cppm`:

```text
client-cache.cppm:33:1: current parser token '{'
instantiating class definition
'std::expected<std::optional<long long>, std::basic_string<char>>'
LLVM IR generation of declaration 'std::is_const_v'
Mangling declaration 'std::is_const_v'
Exception Code: 0xC0000005
```

This crash happened before cache code used `RpcClient`, while parsing local cursor helpers. That is another sign that importing the heavy RPC module can destabilize unrelated importer code.

This strongly suggests a Clang frontend crash in the Windows/MSVC target while importing a C++20/C++23 module that contains Asio coroutine/gRPC template instantiations. It is not a normal C++ diagnostic.

## Current Understanding

The code pattern involved is:

- C++ module interface unit: `client-rpc.cppm`
- Imports `asio_grpc`
- Uses `agrpc::ClientRPC<&SomeService::Stub::PrepareAsyncMethod>`
- Awaits calls through `asio::use_awaitable`
- Other module interface units import `:rpc`

The crash happens in an importing module, not necessarily in the module that directly contains the RPC code. That makes the visible location misleading; for example, `client-bridge.cppm` crashed even when its only RPC dependency was calling `RpcClient::clear_session()`, and `client-session.cppm` later crashed while instantiating unrelated `std::optional<Lockout>` code.

This has reproduced on workflow runs that checked out the expected current commit. Build cache staleness is therefore unlikely to be the root cause, although cache keys should still include compiler, OS image, dependency versions, and relevant CMake/source inputs to avoid mixing incompatible build artifacts.

## Workarounds Tried

1. Replacing explicit-object-parameter member templates with static member templates.
   - Did not help; Clang still crashed mangling `RpcClient::unary_once`.

2. Moving retry/unary helper templates from `RpcClient` into `rpc_detail`.
   - Changed the crashed symbol to `rpc_detail::unary_once`, but did not fix the crash.

3. Replacing helper templates with `std::function` type erasure.
   - Avoided the helper-template symbol, but was rejected because it could add overhead on the RPC hot path.

4. Expanding retry/unary code at call sites with macros while keeping static dispatch.
   - Preserves `agrpc::ClientRPC<&Stub::PrepareAsyncMethod>` static dispatch.
   - Removes `template<auto PrepareAsync>` helper functions from the module interface.
   - The crash then moved into Asio `use_awaitable` mangling when an unrelated module imported `:rpc`.

5. Removing unnecessary `import :rpc` from `client-bridge.cppm`.
   - `NativeBridge` no longer depends directly on `RpcClient`.
   - Force-offline session clearing is passed as `std::move_only_function<void()>`.
   - This reduces the number of modules importing the heavy RPC module and avoids the observed `client-bridge.cppm` crash path.

6. Limiting Ninja compile concurrency for `chatview_client_core` on Windows/Clang.
   - Considered because multiple importing modules crashed near the same build phase.
   - This is a mitigation only; the crash still appears to be a compiler frontend bug.

7. Removing unnecessary `import :rpc` from `client-session.cppm`.
   - `SessionController` now owns only local identity, PIN, and lockout state.
   - It returns `LoginCredentials` to `NativeClient`; `NativeClient` performs the direct `RpcClient::login` / `RpcClient::login_async` calls.
   - This keeps the RPC hot path statically dispatched and avoids `std::function` or `std::move_only_function` on login.
   - `RpcClient::login_async` now cleans its by-value secret-key buffer internally, because the previous caller-side cleanup happened after moving the vector.

8. Removing direct `import :rpc` from `client-cache.cppm` and `client-outbox.cppm`.
   - `CacheController` and `OutboxManager` are now templates over the RPC type.
   - `NativeClient` instantiates them as `CacheController<RpcClient>` and `OutboxManager<RpcClient>`.
   - This keeps calls statically bound and does not add virtual dispatch, `std::function`, or other runtime type erasure on RPC paths.
   - The tradeoff is that the dependent RPC calls instantiate in `client-native.cppm`, which still imports `:rpc`.

## Notes For LLVM Issue

Attach the files emitted by Clang from the failing GitHub Actions run:

```text
C:\Users\RUNNER~1\AppData\Local\Temp\client-bridge-*.cppm
C:\Users\RUNNER~1\AppData\Local\Temp\client-bridge-*.sh
```

If the failure happens in another importing module, attach that pair instead, for example:

```text
C:\Users\RUNNER~1\AppData\Local\Temp\client-session-*.cppm
C:\Users\RUNNER~1\AppData\Local\Temp\client-session-*.sh
```

Useful issue title:

```text
Clang 22.1.6 Windows MSVC target crashes mangling Asio use_awaitable from imported C++23 module
```

Useful labels/components to consider:

- clang
- c++20-modules / c++ modules
- Windows
- coroutines
- crash-on-valid

## MVP Reproducer Plan

Prefer building a standalone MVP before filing the issue. The goal is to prove this is a compiler crash in the template/coroutine/modules shape, not a ChatView, gRPC, or asio-grpc logic error.

Start without gRPC and without asio-grpc. If possible, also start without real Asio and mimic only the relevant `use_awaitable` / `async_result` shape.

Suggested reduction order:

1. Minimal CMake/Ninja/C++23 modules project on Windows with Clang 22.1.6.
2. Module A exports or defines a minimal Asio-like namespace:

```cpp
namespace mini_asio
{
template<class Executor = void>
struct use_awaitable_t
{
};

inline constexpr use_awaitable_t<> use_awaitable{};

template<class CompletionToken, class Signature>
struct async_result;

template<class R>
struct awaitable
{
    struct promise_type
    {
        auto get_return_object() -> awaitable { return {}; }
        auto initial_suspend() noexcept { return std::suspend_never{}; }
        auto final_suspend() noexcept { return std::suspend_never{}; }
        auto return_value(R) noexcept -> void {}
        auto unhandled_exception() -> void {}
    };
};

template<class R>
struct async_result<use_awaitable_t<>, void(R)>
{
    template<class Initiation, class... Args>
    static auto initiate(Initiation&&, use_awaitable_t<>, Args&&...) -> awaitable<R>;
};
}
```

3. Add a gRPC-like status type:

```cpp
struct Status
{
};
```

4. Add an asio-grpc-like `ClientRPC` template with a non-type template parameter that is a member-function pointer:

```cpp
struct Request
{
};

struct Response
{
};

struct Stub
{
    auto PrepareAsyncLogin() -> void;
};

template<auto PrepareAsync>
struct ClientRPC
{
    static auto request(Stub&, Request&, Response&, mini_asio::use_awaitable_t<>) -> mini_asio::awaitable<Status>
    {
        co_return Status{};
    }
};
```

5. Put the RPC-shaped code into a module interface:

```cpp
export module repro.rpc;

import <coroutine>;

export class RpcClient
{
public:
    auto login(this RpcClient& self) -> mini_asio::awaitable<Status>
    {
        Request request;
        Response response;
        co_return co_await ClientRPC<&Stub::PrepareAsyncLogin>::request(
            self.stub_,
            request,
            response,
            mini_asio::use_awaitable);
    }

private:
    Stub stub_;
};
```

6. Add a second module interface that only imports the RPC module:

```cpp
export module repro.bridge;

import repro.rpc;

export struct Bridge
{
    auto f() -> void {}
};
```

7. Compile on Windows with Clang 22.1.6 and Ninja. If this crashes, the LLVM report can avoid all third-party dependencies.

If the synthetic version does not reproduce:

1. Replace `mini_asio` with real standalone Asio, still without gRPC/asio-grpc.
2. Keep the fake `Status`, `Stub`, `Request`, `Response`, and `ClientRPC`.
3. If still no crash, add asio-grpc.
4. Add gRPC last.

The most useful reducer is the smallest version that still crashes while importing the second module. The expected failing shape is:

```text
Clang crashes while mangling async_result<use_awaitable_t<>, void(Status)>::initiate
from an imported C++23 module that contains coroutine/template instantiations.
```

## Current Mitigation In This Repository

`client-bridge.cppm` no longer imports `:rpc`. Instead, `NativeClient` passes a session-clearing callback to `NativeBridge::dispatch_server_event`.

This avoids importing the heavy RPC module into bridge code and keeps the force-offline behavior unchanged.
