module;

#include <array>
#include <exception>
#include <expected>
#include <filesystem>
#include <memory>
#include <span>
#include <string>
#include <string_view>

export module chatview.client:identity;

import :types;
import :detail;
import asio_grpc;

namespace chatview::client
{

class IdentityStore
{
public:
    static auto create(const std::filesystem::path& path) -> std::expected<IdentityStore, std::string>
    {
        return IdentityStore{path};
    }

    auto exists(this const IdentityStore& self) -> bool
    {
        return std::filesystem::exists(self.path_);
    }

    auto create_identity(this const IdentityStore& self, const std::string& pin) -> std::expected<IdentityResult, std::string>
    {
        try {
            std::array<unsigned char, bssl::ed25519_seed_size> seed{};
            if (!bssl::random_bytes(seed)) {
                return std::unexpected{"random generation failed"};
            }
            detail::SecureBufferCleanup seed_cleanup{std::span<unsigned char>{seed}};

            auto keypair = detail::seed_to_keypair(seed);
            if (!keypair) {
                return std::unexpected{keypair.error()};
            }
            detail::SecureBufferCleanup secret_cleanup{std::span<unsigned char>{keypair->second}};

            auto saved = detail::encrypt_seed(self.path_, seed, pin);
            if (!saved) {
                return std::unexpected{saved.error()};
            }

            const auto public_hex = detail::to_hex(keypair->first);
            const auto secret_hex = detail::to_hex(keypair->second);
            return IdentityResult{.publicKey = public_hex, .privateKey = secret_hex};
        } catch (const std::exception& ex) {
            return std::unexpected{std::string{ex.what()}};
        }
    }

    auto import_identity(this const IdentityStore& self, const std::string& private_key_hex, const std::string& pin) -> ExpectedVoid
    {
        try {
            auto seed = detail::seed_from_private_hex(private_key_hex);
            if (!seed) {
                return std::unexpected{seed.error()};
            }
            detail::SecureBufferCleanup seed_cleanup{std::span<unsigned char>{*seed}};
            auto saved = detail::encrypt_seed(self.path_, *seed, pin);
            return saved;
        } catch (const std::exception& ex) {
            return std::unexpected{std::string{ex.what()}};
        }
    }

    auto load_keypair(this const IdentityStore& self, const std::string& pin) -> std::expected<std::pair<detail::ByteVector, detail::ByteVector>, std::string>
    {
        auto seed = detail::decrypt_seed(self.path_, pin);
        if (!seed) {
            return std::unexpected{seed.error()};
        }
        detail::SecureBufferCleanup seed_cleanup{std::span<unsigned char>{*seed}};
        auto keypair = detail::seed_to_keypair(*seed);
        return keypair;
    }

    auto export_private_key(this const IdentityStore& self, const std::string& pin) -> std::expected<std::string, std::string>
    {
        auto keypair = self.load_keypair(pin);
        if (!keypair) {
            return std::unexpected{keypair.error()};
        }
        detail::SecureBufferCleanup secret_cleanup{std::span<unsigned char>{keypair->second}};
        auto secret_hex = detail::to_hex(keypair->second);
        return secret_hex;
    }

private:
    explicit IdentityStore(std::filesystem::path path) : path_(std::move(path)) {}

    std::filesystem::path path_;
};

}
