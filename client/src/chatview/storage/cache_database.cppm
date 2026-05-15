module;

#include <concepts>
#include <cstdint>
#include <exception>
#include <expected>
#include <filesystem>
#include <functional>
#include <memory>
#include <mutex>
#include <optional>
#include <stdexcept>
#include <string>
#include <utility>

#include <sqlite3.h>
#include <sqlite_orm/sqlite_orm.h>

export module chatview.storage.cache;

export namespace chatview::storage
{
struct MessageRow
{
    std::string id;
    std::optional<std::string> client_msg_id;
    std::string peer_pub_key;
    std::string sender_pub_key;
    std::string text;
    std::string timestamp;
    std::optional<std::int64_t> server_seq;
    int delivery = 0;
    std::optional<std::string> error;
    std::string created_at;
};

struct FriendRow
{
    std::string pub_key;
    std::string alias;
    bool is_online = false;
    int unread = 0;
    std::int64_t last_seen_seq = 0;
    std::string updated_at;
};

struct OutboxRow
{
    std::string id;
    std::string receiver_pub_key;
    std::string text;
    int attempts = 0;
    std::optional<std::string> next_retry_at;
    std::optional<std::string> error;
    int status = 0;
    std::string created_at;
};

struct SettingRow
{
    std::string key;
    std::string value;
};

auto make_cache_storage(const std::string& path)
{
    using namespace sqlite_orm;

    return make_storage(
        path,
        make_table(
            "messages",
            make_column("id", &MessageRow::id, primary_key()),
            make_column("client_msg_id", &MessageRow::client_msg_id),
            make_column("peer_pub_key", &MessageRow::peer_pub_key),
            make_column("sender_pub_key", &MessageRow::sender_pub_key),
            make_column("text", &MessageRow::text),
            make_column("timestamp", &MessageRow::timestamp),
            make_column("server_seq", &MessageRow::server_seq),
            make_column("delivery", &MessageRow::delivery, default_value(0)),
            make_column("error", &MessageRow::error),
            make_column("created_at", &MessageRow::created_at, default_value(current_timestamp()))),
        make_table(
            "friends",
            make_column("pub_key", &FriendRow::pub_key, primary_key()),
            make_column("alias", &FriendRow::alias, default_value("")),
            make_column("is_online", &FriendRow::is_online, default_value(false)),
            make_column("unread", &FriendRow::unread, default_value(0)),
            make_column("last_seen_seq", &FriendRow::last_seen_seq, default_value(0)),
            make_column("updated_at", &FriendRow::updated_at, default_value(current_timestamp()))),
        make_table(
            "outbox",
            make_column("id", &OutboxRow::id, primary_key()),
            make_column("receiver_pub_key", &OutboxRow::receiver_pub_key),
            make_column("text", &OutboxRow::text),
            make_column("attempts", &OutboxRow::attempts, default_value(0)),
            make_column("next_retry_at", &OutboxRow::next_retry_at),
            make_column("error", &OutboxRow::error),
            make_column("status", &OutboxRow::status, default_value(0)),
            make_column("created_at", &OutboxRow::created_at, default_value(current_timestamp()))),
        make_table(
            "settings",
            make_column("key", &SettingRow::key, primary_key()),
            make_column("value", &SettingRow::value)));
}

using CacheStorage = decltype(make_cache_storage(std::string{}));

template<typename Function>
concept CacheStorageCallback = std::invocable<Function, CacheStorage&>;

class CacheDatabase
{
public:
    using Storage = CacheStorage;

    CacheDatabase(const CacheDatabase&) = delete;
    CacheDatabase(CacheDatabase&&) noexcept = default;

    auto operator=(const CacheDatabase&) -> CacheDatabase& = delete;
    auto operator=(CacheDatabase&&) -> CacheDatabase& = delete;

    static std::expected<CacheDatabase, std::string> create(const std::filesystem::path& db_path)
    {
        try {
            auto normalized_path = normalize_path(db_path);
            auto& shared = shared_storage(normalized_path);
            if (shared.db_path != normalized_path) {
                return std::unexpected{"cache database already initialized at " + shared.db_path.string()};
            }

            return CacheDatabase{std::move(normalized_path)};
        } catch (const std::exception& ex) {
            return std::unexpected{std::string{ex.what()}};
        }
    }

    template<CacheStorageCallback Function>
    decltype(auto) with_storage(this CacheDatabase& self, Function&& function)
    {
        auto& shared = shared_storage(self.db_path_);
        std::scoped_lock lock{shared.mutex};
        return std::invoke(std::forward<Function>(function), *shared.storage);
    }

    auto path(this const CacheDatabase& self) -> const std::filesystem::path&
    {
        return self.db_path_;
    }

private:
    struct SharedStorage
    {
        std::filesystem::path db_path;
        std::unique_ptr<Storage> storage;
        std::mutex mutex;
    };

    explicit CacheDatabase(std::filesystem::path db_path) : db_path_(std::move(db_path)) {}

    static auto normalize_path(const std::filesystem::path& db_path) -> std::filesystem::path
    {
        return std::filesystem::absolute(db_path).lexically_normal();
    }

    static auto shared_storage(const std::filesystem::path& db_path) -> SharedStorage&
    {
        static auto shared = make_shared_storage(db_path);
        return *shared;
    }

    static auto make_shared_storage(const std::filesystem::path& db_path) -> std::unique_ptr<SharedStorage>
    {
        auto shared = std::make_unique<SharedStorage>();
        shared->db_path = db_path;
        configure_storage(*shared);
        return shared;
    }

    static void configure_storage(SharedStorage& shared)
    {
        if (const auto parent = shared.db_path.parent_path(); !parent.empty()) {
            std::filesystem::create_directories(parent);
        }

        shared.storage.reset(new Storage(make_cache_storage(shared.db_path.string())));
        shared.storage->pragma.busy_timeout(5000);
        shared.storage->pragma.journal_mode(sqlite_orm::journal_mode::WAL);
        shared.storage->sync_schema();
        configure_indexes(shared.db_path);
    }

    static void execute_schema_sql(const std::filesystem::path& db_path, const char* sql)
    {
        sqlite3* db = nullptr;
        if (sqlite3_open(db_path.string().c_str(), &db) != SQLITE_OK) {
            const auto error = db != nullptr ? sqlite3_errmsg(db) : "failed to open sqlite database";
            auto message = std::string{error};
            if (db != nullptr) {
                sqlite3_close(db);
            }
            throw std::runtime_error{message};
        }

        char* raw_error = nullptr;
        if (sqlite3_exec(db, sql, nullptr, nullptr, &raw_error) != SQLITE_OK) {
            auto message = raw_error != nullptr ? std::string{raw_error} : std::string{"sqlite schema statement failed"};
            sqlite3_free(raw_error);
            sqlite3_close(db);
            throw std::runtime_error{message};
        }
        sqlite3_close(db);
    }

    static void configure_indexes(const std::filesystem::path& db_path)
    {
        execute_schema_sql(
            db_path,
            "CREATE INDEX IF NOT EXISTS idx_messages_peer_ts "
            "ON messages(peer_pub_key, timestamp DESC)");
        execute_schema_sql(
            db_path,
            "CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_peer_seq "
            "ON messages(peer_pub_key, server_seq) WHERE server_seq IS NOT NULL");
        execute_schema_sql(
            db_path,
            "CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_client_msg "
            "ON messages(client_msg_id) WHERE client_msg_id IS NOT NULL");
    }

    std::filesystem::path db_path_;
};
}
