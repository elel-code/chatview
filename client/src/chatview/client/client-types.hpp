#pragma once

#include <expected>
#include <filesystem>
#include <optional>
#include <string>
#include <vector>

namespace chatview::client
{

using ExpectedVoid = std::expected<void, std::string>;

struct IdentityResult
{
    std::string publicKey;
    std::string privateKey;
};

struct LoginResult
{
    std::string publicKey;
    int role = 0;
};

struct AuthLockState
{
    std::optional<std::string> lockedUntil;
    std::optional<int> remainingAttempts;
};

struct Friend
{
    std::string pubKey;
    std::string alias;
    bool isOnline = false;
    int unread = 0;
};

struct ChatMessage
{
    std::string id;
    std::string sender;
    std::string text;
    std::string timestamp;
    std::string delivery;
    std::optional<std::string> error;
};

struct SendMessageResult
{
    std::string messageId;
    std::string timestamp;
    std::optional<bool> deduplicated;
};

struct MessageHistoryQuery
{
    std::optional<std::string> cursor;
    std::optional<int> limit;
    std::optional<std::string> direction;
};

struct MessageHistoryPage
{
    std::vector<ChatMessage> messages;
    std::optional<std::string> nextCursor;
    bool hasMore = false;
};

struct UserInfo
{
    std::string pubKey;
    bool isOnline = false;
    bool isBanned = false;
};

struct AdminStats
{
    int onlineUsers = 0;
    int totalUsers = 0;
    int bannedUsers = 0;
};

struct AdminUpdate
{
    std::vector<UserInfo> users;
    AdminStats stats;
};

struct OutboxStatus
{
    int pending = 0;
    int failed = 0;
};

struct NativeClientOptions
{
    std::filesystem::path dataDir;
    std::string grpcTarget = "127.0.0.1:50051";
    std::optional<bool> grpcUseTls;
    std::filesystem::path grpcCaCertPath;
    std::string grpcSslTargetNameOverride;
};

} // namespace chatview::client
