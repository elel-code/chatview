module;

#include <algorithm>
#include <chrono>
#include <expected>
#include <mutex>
#include <optional>
#include <span>
#include <string>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>

export module chatview.client:session;

import :types;
import :detail;
import :identity;
import :rpc;

namespace chatview::client
{

export class SessionController
{
public:
    SessionController(IdentityStore identity, RpcClient& rpc) : identity_(std::move(identity)), rpc_(rpc) {}

    auto has_local_identity(this const SessionController& self) -> bool
    {
        return self.identity_.exists();
    }

    auto create_identity(this SessionController& self, const std::string& pin) -> std::expected<IdentityResult, std::string>
    {
        return self.identity_.create_identity(pin);
    }

    auto import_identity(this SessionController& self, const std::string& private_key, const std::string& pin) -> ExpectedVoid
    {
        return self.identity_.import_identity(private_key, pin);
    }

    auto login(this SessionController& self, const std::string& pin) -> std::expected<LoginResult, std::string>
    {
        if (auto locked = self.current_lock_state(); locked.lockedUntil) {
            return std::unexpected{"too many attempts"};
        }

        auto keypair = self.identity_.load_keypair(pin);
        if (!keypair) {
            self.record_bad_pin();
            return std::unexpected{keypair.error() == "wrong pin" ? "wrong pin" : keypair.error()};
        }

        const auto public_key = detail::to_hex(keypair->first);
        detail::SodiumBufferCleanup secret_cleanup{std::span<unsigned char>{keypair->second}};
        auto result = self.rpc_.login(public_key, keypair->second);

        if (!result) {
            self.record_bad_pin();
            return result;
        }

        {
            std::scoped_lock lock{self.auth_mutex_};
            self.remaining_attempts_ = 5;
            self.locked_until_.reset();
        }
        return result;
    }

    auto login_async(this SessionController& self, const std::string& pin) -> coco::task<std::expected<LoginResult, std::string>>
    {
        if (auto locked = self.current_lock_state(); locked.lockedUntil) {
            co_return std::unexpected{"too many attempts"};
        }

        auto keypair = self.identity_.load_keypair(pin);
        if (!keypair) {
            self.record_bad_pin();
            co_return std::unexpected{keypair.error() == "wrong pin" ? "wrong pin" : keypair.error()};
        }

        const auto public_key = detail::to_hex(keypair->first);
        auto secret_key = std::move(keypair->second);
        detail::SodiumBufferCleanup secret_cleanup{std::span<unsigned char>{secret_key}};
        auto result = co_await self.rpc_.login_async(public_key, std::move(secret_key));

        if (!result) {
            self.record_bad_pin();
            co_return result;
        }

        {
            std::scoped_lock lock{self.auth_mutex_};
            self.remaining_attempts_ = 5;
            self.locked_until_.reset();
        }
        co_return result;
    }

    auto export_private_key(this SessionController& self, const std::string& pin) -> std::expected<std::string, std::string>
    {
        return self.identity_.export_private_key(pin);
    }

    auto lock(this SessionController& self) -> ExpectedVoid
    {
        self.rpc_.clear_session();
        return {};
    }

    auto current_lock_state(this SessionController& self) -> AuthLockState
    {
        std::scoped_lock lock{self.auth_mutex_};
        if (self.locked_until_ && *self.locked_until_ <= std::chrono::system_clock::now()) {
            self.locked_until_.reset();
            self.remaining_attempts_ = 5;
        }

        auto locked_until = std::optional<std::string>{};
        if (self.locked_until_) {
            locked_until = detail::to_iso(*self.locked_until_);
        }

        return AuthLockState{.lockedUntil = std::move(locked_until), .remainingAttempts = self.remaining_attempts_};
    }

private:
    auto record_bad_pin(this SessionController& self) -> void
    {
        std::scoped_lock lock{self.auth_mutex_};
        self.remaining_attempts_ = std::max(0, self.remaining_attempts_ - 1);
        if (self.remaining_attempts_ == 0) {
            self.locked_until_ = std::chrono::system_clock::now() + std::chrono::seconds{30};
        }
    }

    IdentityStore identity_;
    RpcClient& rpc_;
    std::mutex auth_mutex_;
    int remaining_attempts_ = 5;
    std::optional<std::chrono::system_clock::time_point> locked_until_;
};

}
