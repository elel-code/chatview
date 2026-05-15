module;

#include <algorithm>
#include <array>
#include <chrono>
#include <cstddef>
#include <cstdint>
#include <cstdlib>
#include <ctime>
#include <expected>
#include <filesystem>
#include <fstream>
#include <functional>
#include <iterator>
#include <memory>
#include <optional>
#include <span>
#include <sstream>
#include <string>
#include <string_view>
#include <vector>

#include <sodium.h>

export module chatview.client:detail;

import :types;
import asio_grpc;
import chatview.proto.common;
import chatview.storage.cache;

namespace chatview::client::detail
{
using ByteVector = std::vector<unsigned char>;

constexpr std::array<char, 8> identity_magic = {'C', 'H', 'T', 'V', 'I', 'D', '0', '1'};
constexpr auto max_rpc_attempts = 2;

class SodiumBufferCleanup
{
public:
    explicit SodiumBufferCleanup(std::span<unsigned char> bytes) : bytes_{bytes} {}

    SodiumBufferCleanup(const SodiumBufferCleanup&) = delete;
    auto operator=(const SodiumBufferCleanup&) -> SodiumBufferCleanup& = delete;

    ~SodiumBufferCleanup()
    {
        if (!bytes_.empty()) {
            sodium_memzero(bytes_.data(), bytes_.size());
        }
    }

private:
    std::span<unsigned char> bytes_;
};

auto platform_env_string(const char* key) -> std::optional<std::string>
{
    if (const auto* value = std::getenv(key); value != nullptr && value[0] != '\0') {
        return std::string{value};
    }
    return std::nullopt;
}

auto default_data_dir() -> std::filesystem::path
{
#if defined(_WIN32)
    if (auto appdata = platform_env_string("APPDATA")) {
        return std::filesystem::path{*appdata} / "chatview";
    }
#elif defined(__APPLE__)
    if (auto home = platform_env_string("HOME")) {
        return std::filesystem::path{*home} / "Library" / "Application Support" / "chatview";
    }
#endif

    if (auto home = platform_env_string("HOME")) {
        return std::filesystem::path{*home} / ".chatview";
    }
    return std::filesystem::current_path() / ".chatview";
}

auto is_loopback_grpc_target(std::string_view target) -> bool
{
    return target.starts_with("localhost:") ||
           target.starts_with("127.") ||
           target.starts_with("[::1]:") ||
           target.starts_with("::1:");
}

auto default_grpc_use_tls(std::string_view target) -> bool
{
    return !is_loopback_grpc_target(target);
}

auto to_iso(std::chrono::system_clock::time_point time_point) -> std::string
{
    const auto seconds = std::chrono::system_clock::to_time_t(time_point);
    std::tm utc{};
#if defined(_WIN32)
    gmtime_s(&utc, &seconds);
#else
    gmtime_r(&seconds, &utc);
#endif
    std::array<char, 32> buffer{};
    std::strftime(buffer.data(), buffer.size(), "%Y-%m-%dT%H:%M:%SZ", &utc);
    return buffer.data();
}

auto now_iso() -> std::string
{
    return to_iso(std::chrono::system_clock::now());
}

auto to_hex(std::span<const unsigned char> bytes) -> std::string
{
    std::string hex(bytes.size() * 2, '\0');
    sodium_bin2hex(hex.data(), hex.size() + 1, bytes.data(), bytes.size());
    return hex;
}

auto from_hex(std::string_view hex) -> std::expected<ByteVector, std::string>
{
    if (hex.size() % 2 != 0) {
        return std::unexpected{"invalid hex length"};
    }

    ByteVector bytes(hex.size() / 2);
    const auto rc = sodium_hex2bin(
        bytes.data(),
        bytes.size(),
        hex.data(),
        hex.size(),
        nullptr,
        nullptr,
        nullptr);
    if (rc != 0) {
        return std::unexpected{"invalid hex data"};
    }
    return bytes;
}

auto derive_key(std::string_view pin, std::span<const unsigned char> salt) -> std::expected<ByteVector, std::string>
{
    ByteVector key(crypto_secretstream_xchacha20poly1305_KEYBYTES);
    const auto rc = crypto_pwhash(
        key.data(),
        key.size(),
        pin.data(),
        pin.size(),
        salt.data(),
        crypto_pwhash_OPSLIMIT_MODERATE,
        crypto_pwhash_MEMLIMIT_MODERATE,
        crypto_pwhash_ALG_ARGON2ID13);
    if (rc != 0) {
        return std::unexpected{"key derivation failed"};
    }
    return key;
}

auto read_file(const std::filesystem::path& path) -> std::expected<ByteVector, std::string>
{
    std::ifstream input{path, std::ios::binary};
    if (!input) {
        return std::unexpected{"failed to open: " + path.string()};
    }

    const auto begin = input.tellg();
    input.seekg(0, std::ios::end);
    const auto end = input.tellg();
    input.seekg(0, std::ios::beg);

    ByteVector bytes(static_cast<std::size_t>(end - begin));
    if (!bytes.empty()) {
        input.read(reinterpret_cast<char*>(bytes.data()), static_cast<std::streamsize>(bytes.size()));
    }

    if (!input) {
        return std::unexpected{"failed to read: " + path.string()};
    }
    return bytes;
}

auto write_file(const std::filesystem::path& path, std::span<const unsigned char> bytes) -> ExpectedVoid
{
    if (const auto parent = path.parent_path(); !parent.empty()) {
        std::filesystem::create_directories(parent);
    }

    std::ofstream output{path, std::ios::binary};
    if (!output) {
        return std::unexpected{"failed to open for writing: " + path.string()};
    }
    output.write(reinterpret_cast<const char*>(bytes.data()), static_cast<std::streamsize>(bytes.size()));

    if (!output) {
        return std::unexpected{"failed to write: " + path.string()};
    }
    return {};
}

auto encrypt_seed(
    const std::filesystem::path& path,
    std::span<const unsigned char> seed,
    std::string_view pin) -> ExpectedVoid
{
    constexpr auto salt_size = crypto_pwhash_SALTBYTES;
    constexpr auto header_size = crypto_secretstream_xchacha20poly1305_HEADERBYTES;
    constexpr auto abytes = crypto_secretstream_xchacha20poly1305_ABYTES;

    std::array<unsigned char, salt_size> salt{};
    randombytes_buf(salt.data(), salt.size());

    auto key = derive_key(pin, salt);
    if (!key) {
        return std::unexpected{key.error()};
    }

    crypto_secretstream_xchacha20poly1305_state state;
    std::array<unsigned char, header_size> header{};
    crypto_secretstream_xchacha20poly1305_init_push(&state, header.data(), key->data());

    ByteVector cipher(seed.size() + abytes);
    unsigned long long cipher_size = 0;
    crypto_secretstream_xchacha20poly1305_push(
        &state,
        cipher.data(),
        &cipher_size,
        seed.data(),
        static_cast<unsigned long long>(seed.size()),
        nullptr,
        0,
        crypto_secretstream_xchacha20poly1305_TAG_FINAL);

    sodium_memzero(key->data(), key->size());

    ByteVector payload;
    payload.reserve(identity_magic.size() + salt_size + header_size + cipher_size);
    payload.insert(payload.end(), identity_magic.begin(), identity_magic.end());
    payload.insert(payload.end(), salt.begin(), salt.end());
    payload.insert(payload.end(), header.begin(), header.end());
    payload.insert(payload.end(), cipher.begin(), cipher.begin() + static_cast<std::ptrdiff_t>(cipher_size));

    return write_file(path, payload);
}

auto decrypt_seed(const std::filesystem::path& path, std::string_view pin) -> std::expected<ByteVector, std::string>
{
    auto file = read_file(path);
    if (!file) {
        return std::unexpected{file.error()};
    }

    constexpr auto salt_size = crypto_pwhash_SALTBYTES;
    constexpr auto header_size = crypto_secretstream_xchacha20poly1305_HEADERBYTES;
    constexpr auto abytes = crypto_secretstream_xchacha20poly1305_ABYTES;

    constexpr auto min_size = identity_magic.size() + salt_size + header_size + abytes;

    if (file->size() < min_size) {
        return std::unexpected{"invalid identity file"};
    }

    if (!std::equal(identity_magic.begin(), identity_magic.end(), file->begin())) {
        return std::unexpected{"invalid identity header"};
    }

    auto it = file->begin() + static_cast<std::ptrdiff_t>(identity_magic.size());
    const auto* salt = &*it;
    it += static_cast<std::ptrdiff_t>(salt_size);
    const auto* header = &*it;
    it += static_cast<std::ptrdiff_t>(header_size);
    const auto* encrypted_begin = &*it;
    const auto encrypted_size = static_cast<std::size_t>(file->end() - it);

    auto key = derive_key(pin, {salt, salt_size});
    if (!key) {
        return std::unexpected{key.error()};
    }

    crypto_secretstream_xchacha20poly1305_state state;
    if (crypto_secretstream_xchacha20poly1305_init_pull(&state, header, key->data()) != 0) {
        sodium_memzero(key->data(), key->size());
        return std::unexpected{"invalid identity header"};
    }

    ByteVector seed(static_cast<std::size_t>(encrypted_size));
    unsigned long long seed_size = 0;
    unsigned char tag = 0;
    const auto rc = crypto_secretstream_xchacha20poly1305_pull(
        &state,
        seed.data(),
        &seed_size,
        &tag,
        encrypted_begin,
        static_cast<unsigned long long>(encrypted_size),
        nullptr,
        0);

    sodium_memzero(key->data(), key->size());
    sodium_memzero(&state, sizeof(state));

    if (rc != 0 || tag != crypto_secretstream_xchacha20poly1305_TAG_FINAL) {
        return std::unexpected{"wrong pin"};
    }
    seed.resize(static_cast<std::size_t>(seed_size));
    if (seed.size() != crypto_sign_SEEDBYTES) {
        return std::unexpected{"invalid identity payload"};
    }
    return seed;
}

auto seed_to_keypair(std::span<const unsigned char> seed) -> std::expected<std::pair<ByteVector, ByteVector>, std::string>
{
    if (seed.size() != crypto_sign_SEEDBYTES) {
        return std::unexpected{"invalid private key size"};
    }

    ByteVector public_key(crypto_sign_PUBLICKEYBYTES);
    ByteVector secret_key(crypto_sign_SECRETKEYBYTES);
    crypto_sign_seed_keypair(public_key.data(), secret_key.data(), seed.data());
    return std::pair{std::move(public_key), std::move(secret_key)};
}

auto seed_from_private_hex(std::string_view private_key_hex) -> std::expected<ByteVector, std::string>
{
    auto bytes = from_hex(private_key_hex);
    if (!bytes) {
        return std::unexpected{bytes.error()};
    }
    if (bytes->size() == crypto_sign_SEEDBYTES) {
        return bytes;
    }
    if (bytes->size() == crypto_sign_SECRETKEYBYTES) {
        ByteVector seed(bytes->begin(), bytes->begin() + crypto_sign_SEEDBYTES);
        sodium_memzero(bytes->data(), bytes->size());
        return seed;
    }
    return std::unexpected{"private key must be 32-byte seed or 64-byte Ed25519 secret key hex"};
}

auto grpc_error(const grpc::Status& status) -> std::string
{
    switch (status.error_code()) {
    case grpc::StatusCode::PERMISSION_DENIED:
        return status.error_message().empty() ? "permission denied" : "permission denied: " + status.error_message();
    case grpc::StatusCode::UNAUTHENTICATED:
        return status.error_message().empty() ? "unauthenticated" : "unauthenticated: " + status.error_message();
    case grpc::StatusCode::UNAVAILABLE:
        return status.error_message().empty() ? "service unavailable" : "service unavailable: " + status.error_message();
    case grpc::StatusCode::DEADLINE_EXCEEDED:
        return status.error_message().empty() ? "request timed out" : "request timed out: " + status.error_message();
    case grpc::StatusCode::INVALID_ARGUMENT:
        return status.error_message().empty() ? "invalid argument" : "invalid argument: " + status.error_message();
    case grpc::StatusCode::NOT_FOUND:
        return status.error_message().empty() ? "not found" : "not found: " + status.error_message();
    case grpc::StatusCode::ALREADY_EXISTS:
        return status.error_message().empty() ? "already exists" : "already exists: " + status.error_message();
    default:
        break;
    }

    std::ostringstream out;
    out << "grpc error " << static_cast<int>(status.error_code()) << ": " << status.error_message();
    return out.str();
}

auto delivery_to_string(int delivery) -> std::string
{
    using namespace chatview::proto::common;
    switch (delivery) {
    case MESSAGE_DELIVERY_INCOMING:
        return "incoming";
    case MESSAGE_DELIVERY_PENDING:
        return "pending";
    case MESSAGE_DELIVERY_SENT:
        return "sent";
    case MESSAGE_DELIVERY_FAILED:
        return "failed";
    default:
        return "pending";
    }
}

auto string_to_delivery(std::string_view delivery) -> int
{
    using namespace chatview::proto::common;
    if (delivery == "incoming") {
        return MESSAGE_DELIVERY_INCOMING;
    }
    if (delivery == "sent") {
        return MESSAGE_DELIVERY_SENT;
    }
    if (delivery == "failed") {
        return MESSAGE_DELIVERY_FAILED;
    }
    return MESSAGE_DELIVERY_PENDING;
}

auto js_string(std::string_view input) -> std::string
{
    constexpr std::array<char, 16> hex = {'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'};

    std::string escaped;
    escaped.reserve(input.size() + 2);
    escaped += '"';
    for (const auto ch : input) {
        const auto byte = static_cast<unsigned char>(ch);
        switch (ch) {
        case '\\':
            escaped += "\\\\";
            break;
        case '"':
            escaped += "\\\"";
            break;
        case '\b':
            escaped += "\\b";
            break;
        case '\f':
            escaped += "\\f";
            break;
        case '\n':
            escaped += "\\n";
            break;
        case '\r':
            escaped += "\\r";
            break;
        case '\t':
            escaped += "\\t";
            break;
        default:
            if (byte < 0x20) {
                escaped += "\\u00";
                escaped += hex[(byte >> 4) & 0x0f];
                escaped += hex[byte & 0x0f];
            } else {
                escaped += ch;
            }
            break;
        }
    }
    escaped += '"';
    return escaped;
}

auto message_from_row(const storage::MessageRow& row) -> ChatMessage
{
    return ChatMessage{
        .id = row.id,
        .sender = row.sender_pub_key,
        .text = row.text,
        .timestamp = row.timestamp,
        .delivery = delivery_to_string(row.delivery),
        .error = row.error,
    };
}

auto friend_from_row(const storage::FriendRow& row) -> Friend
{
    return Friend{
        .pubKey = row.pub_key,
        .alias = row.alias,
        .isOnline = row.is_online,
        .unread = row.unread,
    };
}

auto message_from_proto(const chatview::proto::common::ChatMessage& message, const std::string& peer_pub_key) -> storage::MessageRow
{
    auto error = std::optional<std::string>{};
    if (!message.error().empty()) {
        error = message.error();
    }
    return storage::MessageRow{
        .id = message.id(),
        .client_msg_id = std::nullopt,
        .peer_pub_key = peer_pub_key,
        .sender_pub_key = message.sender_pub_key(),
        .text = message.text(),
        .timestamp = message.timestamp(),
        .server_seq = message.server_seq() > 0 ? std::optional<std::int64_t>{message.server_seq()} : std::nullopt,
        .delivery = static_cast<int>(message.delivery()),
        .error = std::move(error),
        .created_at = now_iso(),
    };
}

auto build_page(std::vector<storage::MessageRow> rows, int limit, bool ascending) -> MessageHistoryPage
{
    if (ascending) {
        std::ranges::sort(rows, {}, [](const storage::MessageRow& row) { return row.server_seq.value_or(0); });
    } else {
        std::ranges::sort(rows, std::greater<>{}, [](const storage::MessageRow& row) { return row.server_seq.value_or(0); });
    }

    const auto has_more = static_cast<int>(rows.size()) > limit;
    if (has_more) {
        rows.resize(static_cast<std::size_t>(limit));
    }

    auto next_cursor = std::optional<std::string>{};
    if (!rows.empty() && rows.back().server_seq) {
        next_cursor = std::to_string(*rows.back().server_seq);
    }

    std::vector<ChatMessage> messages;
    messages.reserve(rows.size());
    std::ranges::transform(rows, std::back_inserter(messages), message_from_row);
    return MessageHistoryPage{.messages = std::move(messages), .nextCursor = std::move(next_cursor), .hasMore = has_more};
}

auto limit_or_default(std::optional<int> limit) -> int
{
    return std::clamp(limit.value_or(30), 1, 100);
}

auto direction_or_default(const MessageHistoryQuery& query) -> std::string
{
    return query.direction.value_or("older");
}

}
