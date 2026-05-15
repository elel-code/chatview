#include <cassert>
#include <chrono>
#include <cstdlib>
#include <iostream>
#include <filesystem>
#include <fstream>
#include <expected>
#include <optional>
#include <string>
#include <utility>

#include <glaze/glaze.hpp>
#include <sqlite_orm/sqlite_orm.h>

import chatview.client;
import chatview.storage.cache;

namespace
{
template<typename T, typename Error>
auto require_expected(std::expected<T, Error>&& value, const char* context) -> T
{
    if (!value) {
        std::cerr << context << ": " << value.error() << '\n';
        std::abort();
    }
    return std::move(*value);
}

template<typename Error>
void require_expected(std::expected<void, Error>&& value, const char* context)
{
    if (!value) {
        std::cerr << context << ": " << value.error() << '\n';
        std::abort();
    }
}

auto test_root() -> std::filesystem::path
{
    const auto suffix = std::chrono::steady_clock::now().time_since_epoch().count();
    return std::filesystem::temp_directory_path() / ("chatview-client-local-tests-" + std::to_string(suffix));
}

void write_text(const std::filesystem::path& path, const std::string& text)
{
    std::ofstream output{path, std::ios::binary};
    assert(output);
    output << text;
    assert(output);
}

void identity_and_pin_lock_test(const std::filesystem::path& root)
{
    auto options = chatview::client::default_options();
    options.dataDir = root / "native";
    options.grpcTarget = "127.0.0.1:1";
    options.grpcUseTls = false;

    auto native = require_expected(chatview::client::NativeClient::create(options, [](std::string) {}), "NativeClient::create");
    auto& client = *native;

    assert(!client.hasLocalIdentity());

    auto created = require_expected(client.createIdentity("123456"), "createIdentity");
    assert(!created.publicKey.empty());
    assert(!created.privateKey.empty());
    assert(client.hasLocalIdentity());

    auto wrong_export = client.exportPrivateKey("000000");
    assert(!wrong_export);

    auto exported = require_expected(client.exportPrivateKey("123456"), "exportPrivateKey");
    assert(exported == created.privateKey);

    for (int attempt = 0; attempt < 5; ++attempt) {
        auto login = client.login("000000");
        assert(!login);
        assert(login.error() == "wrong pin");
    }
    const auto locked = client.getAuthLockState();
    assert(locked.lockedUntil);
    assert(locked.remainingAttempts == 0);

    auto locked_login = client.login("123456");
    assert(!locked_login);
    assert(locked_login.error() == "too many attempts");

    require_expected(client.importIdentity(created.privateKey, "654321"), "importIdentity");
    auto reexported = require_expected(client.exportPrivateKey("654321"), "exportPrivateKey after import");
    assert(reexported == created.privateKey);

    write_text(options.dataDir / "identity.bin", "not-a-chatview-identity");
    auto corrupt_export = client.exportPrivateKey("654321");
    assert(!corrupt_export);
}

void cache_and_outbox_test(const std::filesystem::path& root)
{
    using namespace sqlite_orm;

    auto cache = require_expected(chatview::storage::CacheDatabase::create(root / "native" / "cache.db"), "CacheDatabase::create");

    cache.with_storage([](auto& storage) {
        storage.replace(chatview::storage::MessageRow{
            .id = "local-1",
            .client_msg_id = "client-1",
            .peer_pub_key = "peer-a",
            .sender_pub_key = "me",
            .text = "hello",
            .timestamp = "2026-05-15T00:00:00Z",
            .server_seq = std::nullopt,
            .delivery = 3,
            .error = "offline",
            .created_at = "2026-05-15T00:00:00Z",
        });
        storage.replace(chatview::storage::OutboxRow{
            .id = "client-1",
            .receiver_pub_key = "peer-a",
            .text = "hello",
            .attempts = 2,
            .next_retry_at = "2026-05-15T00:05:00Z",
            .error = "unavailable",
            .status = 2,
            .created_at = "2026-05-15T00:00:00Z",
        });

        const auto failed = storage.template count<chatview::storage::OutboxRow>(
            where(is_equal(&chatview::storage::OutboxRow::status, 2)));
        assert(failed == 1);

        auto rows = storage.template get_all<chatview::storage::OutboxRow>(
            where(is_equal(&chatview::storage::OutboxRow::id, "client-1")));
        assert(rows.size() == 1);
        assert(rows.front().error == "unavailable");

        rows.front().status = 0;
        rows.front().error = std::nullopt;
        rows.front().next_retry_at = std::nullopt;
        storage.update(rows.front());

        const auto pending = storage.template count<chatview::storage::OutboxRow>(
            where(is_equal(&chatview::storage::OutboxRow::status, 0)));
        assert(pending == 1);
    });
}

void dto_json_test()
{
    chatview::client::ChatMessage message{
        .id = "m1",
        .sender = "alice",
        .text = "quote \" and newline\n",
        .timestamp = "2026-05-15T00:00:00Z",
        .delivery = "failed",
        .error = "network",
    };

    std::string json;
    auto write_error = glz::write_json(message, json);
    assert(!write_error);
    assert(json.find("\"id\":\"m1\"") != std::string::npos);
    assert(json.find("\"sender\":\"alice\"") != std::string::npos);
    assert(json.find("\"delivery\":\"failed\"") != std::string::npos);
    assert(json.find("\"error\":\"network\"") != std::string::npos);

    chatview::client::ChatMessage parsed;
    auto read_error = glz::read_json(parsed, json);
    assert(!read_error);
    assert(parsed.id == message.id);
    assert(parsed.sender == message.sender);
    assert(parsed.text == message.text);
    assert(parsed.timestamp == message.timestamp);
    assert(parsed.delivery == message.delivery);
    assert(parsed.error == message.error);

    chatview::client::OutboxStatus outbox{.pending = 2, .failed = 1};
    json.clear();
    write_error = glz::write_json(outbox, json);
    assert(!write_error);
    assert(json.find("\"pending\":2") != std::string::npos);
    assert(json.find("\"failed\":1") != std::string::npos);
}

}

int main()
{
    const auto root = test_root();
    std::filesystem::remove_all(root);
    std::filesystem::create_directories(root);

    identity_and_pin_lock_test(root);
    cache_and_outbox_test(root);
    dto_json_test();

    std::filesystem::remove_all(root);
}
