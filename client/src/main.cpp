#include <cstdint>
#include <expected>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <memory>
#include <optional>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

#include <saucer/embedded/all.hpp>
#include <saucer/smartview.hpp>
#include <glaze/yaml.hpp>

import chatview.client;

namespace chatview::client_config
{
struct File
{
    std::string dataDir;
    std::string grpcTarget;
    std::optional<bool> grpcUseTls;
    std::string grpcCaCertPath;
    std::string grpcSslTargetNameOverride;
};
}

template <>
struct glz::meta<chatview::client_config::File>
{
    using T = chatview::client_config::File;
    static constexpr auto value = object(
        "data_dir", &T::dataDir,
        "grpc_target", &T::grpcTarget,
        "grpc_tls", &T::grpcUseTls,
        "grpc_ca_path", &T::grpcCaCertPath,
        "grpc_ssl_target_name_override", &T::grpcSslTargetNameOverride);
};

namespace
{
struct LaunchOptions
{
    std::filesystem::path config_path;
    bool devtools = false;
    bool no_native = false;
    std::string startup = "app";
    std::string startup_url;
    std::vector<std::string> saucer_args;
};

auto parse_launch_options(int argc, char** argv) -> std::expected<LaunchOptions, std::string>
{
    LaunchOptions options;
    if (argc > 0) {
        options.saucer_args.emplace_back(argv[0]);
    }
    for (int i = 1; i < argc; ++i) {
        const std::string_view arg{argv[i]};
        if (arg == "--config") {
            if (i + 1 >= argc) {
                return std::unexpected{"--config requires a YAML file path"};
            }
            options.config_path = argv[++i];
            continue;
        }
        constexpr std::string_view prefix = "--config=";
        if (arg.starts_with(prefix)) {
            options.config_path = std::string{arg.substr(prefix.size())};
            continue;
        }
        if (arg == "--devtools") {
            options.devtools = true;
            continue;
        }
        if (arg == "--no-native") {
            options.no_native = true;
            continue;
        }
        if (arg == "--startup") {
            if (i + 1 >= argc) {
                return std::unexpected{"--startup requires one of: app, embedded, serve, html, url"};
            }
            options.startup = argv[++i];
            continue;
        }
        constexpr std::string_view startup_prefix = "--startup=";
        if (arg.starts_with(startup_prefix)) {
            options.startup = std::string{arg.substr(startup_prefix.size())};
            continue;
        }
        if (arg == "--url") {
            if (i + 1 >= argc) {
                return std::unexpected{"--url requires a URL"};
            }
            options.startup = "url";
            options.startup_url = argv[++i];
            continue;
        }
        constexpr std::string_view url_prefix = "--url=";
        if (arg.starts_with(url_prefix)) {
            options.startup = "url";
            options.startup_url = std::string{arg.substr(url_prefix.size())};
            continue;
        }
        options.saucer_args.emplace_back(argv[i]);
    }
    if (options.startup != "app" && options.startup != "embedded" && options.startup != "serve" && options.startup != "html" &&
        options.startup != "url") {
        return std::unexpected{"--startup must be one of: app, embedded, serve, html, url"};
    }
    if (options.startup == "url" && options.startup_url.empty()) {
        return std::unexpected{"--startup=url requires --url"};
    }
    return options;
}

auto load_options(const std::filesystem::path& config_path) -> std::expected<chatview::client::NativeClientOptions, std::string>
{
    auto options = chatview::client::default_options();
    if (config_path.empty()) {
        return options;
    }

    std::ifstream input{config_path, std::ios::binary};
    if (!input) {
        return std::unexpected{"failed to open config: " + config_path.string()};
    }

    std::string yaml{
        std::istreambuf_iterator<char>{input},
        std::istreambuf_iterator<char>{}};
    if (!input.eof() && input.fail()) {
        return std::unexpected{"failed to read config: " + config_path.string()};
    }

    chatview::client_config::File file_config{};
    if (auto error = glz::read_yaml(file_config, yaml)) {
        return std::unexpected{"failed to parse config " + config_path.string() + ": " + glz::format_error(error, yaml)};
    }

    if (!file_config.dataDir.empty()) {
        options.dataDir = std::move(file_config.dataDir);
    }
    if (!file_config.grpcTarget.empty()) {
        options.grpcTarget = std::move(file_config.grpcTarget);
    }
    if (file_config.grpcUseTls) {
        options.grpcUseTls = *file_config.grpcUseTls;
    }
    if (!file_config.grpcCaCertPath.empty()) {
        options.grpcCaCertPath = std::move(file_config.grpcCaCertPath);
    }
    if (!file_config.grpcSslTargetNameOverride.empty()) {
        options.grpcSslTargetNameOverride = std::move(file_config.grpcSslTargetNameOverride);
    }
    return options;
}

}

int main(int argc, char** argv)
{
    auto launch_options = parse_launch_options(argc, argv);
    if (!launch_options) {
        std::cerr << launch_options.error() << '\n';
        return 1;
    }

    std::optional<chatview::client::NativeClientOptions> native_options;
    if (!launch_options->no_native) {
        auto loaded_options = load_options(launch_options->config_path);
        if (!loaded_options) {
            std::cerr << loaded_options.error() << '\n';
            return 1;
        }
        native_options = std::move(*loaded_options);
    }

    std::vector<char*> saucer_argv;
    saucer_argv.reserve(launch_options->saucer_args.size());
    for (auto& arg : launch_options->saucer_args) {
        saucer_argv.push_back(arg.data());
    }

    auto app = saucer::application::create({
        .id = "chatview.client",
        .argc = static_cast<int>(saucer_argv.size()),
        .argv = saucer_argv.data(),
    });
    if (!app) {
        return 1;
    }
    return app->run([options = std::move(native_options),
                        devtools = launch_options->devtools,
                        startup = launch_options->startup,
                        startup_url = launch_options->startup_url](saucer::application* app) mutable -> coco::stray {
        auto window = saucer::window::create(app).value();
        auto webview = saucer::smartview::create({.window = window});
        auto* view = std::addressof(*webview);
        webview->set_dev_tools(devtools);

        auto dispatcher = [app, view](std::string script) {
            app->post([view, script = std::move(script)] mutable {
                view->saucer::webview::execute(script);
            });
        };

        if (options) {
            auto native = chatview::client::NativeClient::create(std::move(*options), std::move(dispatcher));
            if (!native) {
                webview->set_html("<!doctype html><body><pre>Native client startup failed: " + native.error() + "</pre></body>");
                window->show();
                co_await app->finish();
                co_return;
            }

            auto client = std::shared_ptr<chatview::client::NativeClient>{std::move(*native)};

            webview->expose("hasLocalIdentity", [client] {
                return client->hasLocalIdentity();
            });
            webview->expose("createIdentity", [client](std::string pin) {
                return client->createIdentity(pin);
            });
            webview->expose("importIdentity", [client](std::string private_key, std::string new_pin) {
                return client->importIdentity(private_key, new_pin);
            });
            webview->expose("login", [client](std::string pin) -> coco::task<std::expected<chatview::client::LoginResult, std::string>> {
                co_return co_await client->loginAsync(std::move(pin));
            });
            webview->expose("exportPrivateKey", [client](std::string pin) {
                return client->exportPrivateKey(pin);
            });
            webview->expose("lockSession", [client] {
                return client->lockSession();
            });
            webview->expose("getAuthLockState", [client] {
                return client->getAuthLockState();
            });
            webview->expose("listFriends", [client]() -> coco::task<std::expected<std::vector<chatview::client::Friend>, std::string>> {
                co_return co_await client->listFriendsAsync();
            });
            webview->expose("getMessageHistory", [client](
                std::string pub_key,
                chatview::client::MessageHistoryQuery query) -> coco::task<std::expected<chatview::client::MessageHistoryPage, std::string>> {
                co_return co_await client->getMessageHistoryAsync(std::move(pub_key), std::move(query));
            });
            webview->expose("sendMessage", [client](
                std::string receiver_pub_key,
                std::string text,
                std::string client_message_id) -> coco::task<std::expected<chatview::client::SendMessageResult, std::string>> {
                co_return co_await client->sendMessageAsync(
                    std::move(receiver_pub_key),
                    std::move(text),
                    std::move(client_message_id));
            });
            webview->expose("markConversationRead", [client](
                std::string pub_key,
                std::optional<std::int64_t> last_read_server_seq) -> coco::task<chatview::client::ExpectedVoid> {
                co_return co_await client->markConversationReadAsync(std::move(pub_key), last_read_server_seq);
            });
            webview->expose("addFriend", [client](std::string target_pub_key) -> coco::task<chatview::client::ExpectedVoid> {
                co_return co_await client->addFriendAsync(std::move(target_pub_key));
            });
            webview->expose("adminSetUserStatus", [client](
                std::string target_pub_key,
                std::string status) -> coco::task<chatview::client::ExpectedVoid> {
                co_return co_await client->adminSetUserStatusAsync(std::move(target_pub_key), std::move(status));
            });
            webview->expose("adminBroadcast", [client](std::string text) -> coco::task<chatview::client::ExpectedVoid> {
                co_return co_await client->adminBroadcastAsync(std::move(text));
            });
            webview->expose("pollAdminEvents", [client]() -> coco::task<std::expected<chatview::client::AdminUpdate, std::string>> {
                co_return co_await client->pollAdminEventsAsync();
            });
            webview->expose("getOutboxStatus", [client] {
                return client->getOutboxStatus();
            });
            webview->expose("retryOutboxMessage", [client](std::string message_id) {
                return client->retryOutboxMessage(message_id);
            });
            webview->expose("clearOutbox", [client] {
                return client->clearOutbox();
            });
        }

        window->set_title("ChatView");
        window->set_size({.w = 1200, .h = 800});

        webview->embed(saucer::embedded::all());
        if (startup == "html") {
            webview->set_html("<!doctype html><title>ChatView smoke</title><body><h1>ChatView WebView OK</h1><p>set_html startup passed.</p></body>");
        } else if (startup == "serve") {
            webview->serve("/index.html");
        } else if (startup == "embedded") {
            webview->set_url("saucer://embedded/index.html");
        } else if (startup == "url") {
            webview->set_url(startup_url);
        } else {
            webview->set_url("saucer://embedded/index.html#/auth");
        }

        window->show();
        co_await app->finish();
    });
}
