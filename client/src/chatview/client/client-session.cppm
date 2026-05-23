module;

#include <algorithm>
#include <chrono>
#include <expected>
#include <mutex>
#include <optional>
#include <string>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>

export module chatview.client:session;

import :types;
import :detail;
import :identity;

namespace chatview::client
{

export struct LoginCredentials
{
    std::string publicKey;
    std::vector<unsigned char> secretKey;
};

export class SessionController
{
public:
    explicit SessionController(IdentityStore identity) : identity_(std::move(identity)) {}

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

    auto load_login_credentials(this SessionController& self, const std::string& pin) -> std::expected<LoginCredentials, std::string>
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
        return LoginCredentials{.publicKey = public_key, .secretKey = std::move(keypair->second)};
    }

    auto load_login_credentials_async(this SessionController& self, const std::string& pin) -> coco::task<std::expected<LoginCredentials, std::string>>
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
        co_return LoginCredentials{.publicKey = public_key, .secretKey = std::move(keypair->second)};
    }

    auto record_login_success(this SessionController& self) -> void
    {
        {
            std::scoped_lock lock{self.auth_mutex_};
            self.remaining_attempts_ = 5;
            self.locked_until_ = {};
        }
    }

    auto record_login_failure(this SessionController& self) -> void
    {
        self.record_bad_pin();
    }

    auto export_private_key(this SessionController& self, const std::string& pin) -> std::expected<std::string, std::string>
    {
        return self.identity_.export_private_key(pin);
    }

    auto lock(this SessionController& self) -> ExpectedVoid
    {
        return {};
    }

    auto current_lock_state(this SessionController& self) -> AuthLockState
    {
        std::scoped_lock lock{self.auth_mutex_};
        if (self.locked_until_ && self.locked_until_->steady_until <= std::chrono::steady_clock::now()) {
            self.locked_until_ = {};
            self.remaining_attempts_ = 5;
        }

        auto locked_until = std::optional<std::string>{};
        if (self.locked_until_) {
            locked_until = detail::to_iso(self.locked_until_->system_until);
        }

        return AuthLockState{.lockedUntil = std::move(locked_until), .remainingAttempts = self.remaining_attempts_};
    }

private:
    auto record_bad_pin(this SessionController& self) -> void
    {
        std::scoped_lock lock{self.auth_mutex_};
        self.remaining_attempts_ = std::max(0, self.remaining_attempts_ - 1);
        if (self.remaining_attempts_ == 0) {
            self.locked_until_ = Lockout{
                .steady_until = std::chrono::steady_clock::now() + std::chrono::seconds{30},
                .system_until = std::chrono::system_clock::now() + std::chrono::seconds{30},
            };
        }
    }

    struct Lockout
    {
        std::chrono::steady_clock::time_point steady_until;
        std::chrono::system_clock::time_point system_until;
    };

    IdentityStore identity_;
    std::mutex auth_mutex_;
    int remaining_attempts_ = 5;
    std::optional<Lockout> locked_until_;
};

}
