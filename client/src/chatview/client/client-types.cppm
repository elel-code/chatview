module;

#include <expected>
#include <filesystem>
#include <optional>
#include <string>
#include <vector>

export module chatview.client:types;

namespace chatview::client
{
export using ExpectedVoid = std::expected<void, std::string>;

export struct IdentityResult
{
    std::string publicKey;
    std::string privateKey;
};

export struct LoginResult
{
    std::string publicKey;
    int role = 0;
};

export struct AuthLockState
{
    std::optional<std::string> lockedUntil;
    std::optional<int> remainingAttempts;
};

export struct Friend
{
    std::string pubKey;
    std::string alias;
    bool isOnline = false;
    int unread = 0;
};

export struct ChatMessage
{
    std::string id;
    std::string sender;
    std::string text;
    std::string timestamp;
    std::string delivery;
    std::optional<std::string> error;
};

export struct SendMessageResult
{
    std::string messageId;
    std::string timestamp;
    std::optional<bool> deduplicated;
};

export struct MessageHistoryQuery
{
    std::optional<std::string> cursor;
    std::optional<int> limit;
    std::optional<std::string> direction;
};

export struct MessageHistoryPage
{
    std::vector<ChatMessage> messages;
    std::optional<std::string> nextCursor;
    bool hasMore = false;
};

export struct UserInfo
{
    std::string pubKey;
    bool isOnline = false;
    bool isBanned = false;
};

export struct AdminStats
{
    int onlineUsers = 0;
    int totalUsers = 0;
    int bannedUsers = 0;
};

export struct AdminUpdate
{
    std::vector<UserInfo> users;
    AdminStats stats;
};

export struct OutboxStatus
{
    int pending = 0;
    int failed = 0;
};

export struct NativeClientOptions
{
    std::filesystem::path dataDir;
    std::string grpcTarget = "127.0.0.1:50051";
    std::optional<bool> grpcUseTls;
    std::filesystem::path grpcCaCertPath;
    std::string grpcSslTargetNameOverride;
};
}
