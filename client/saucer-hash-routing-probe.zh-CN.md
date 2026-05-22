# Saucer Hash 路由重复资源请求探测

## 目的

这份文档用于定位嵌入式宿主中 hash 路由启动跳转时的重复资源请求问题。

当前已知现象：

- 初始地址为 `/index.html`，路由守卫跳转到 `/auth` 后，宿主侧会看到两次 HTML 资源请求。
- 初始地址为 `/index.html#/`，路由守卫跳转到 `/auth` 后，也会看到两次 HTML 资源请求。
- 初始地址为 `/index.html#/auth` 时，不会出现额外请求。
- 应用加载完成后的后续 `navigation.navigate(...)` 没有这个问题。

探测的目标不是立即改变路由行为，而是确认第二次请求来自哪里：

- 是真正的 top-level document navigation。
- 是 custom scheme / embedded asset handler 被触发。
- 是页面内 Navigation API 的 same-document hash navigation 被宿主误识别。
- 还是启动期 document 尚未稳定时的 WebView backend 行为。

只有确认触发点后，才能决定是否需要在路由启动阶段做特殊处理。

## 宿主层探测

在 Saucer webview 上挂 `navigate`、`navigated`、`request`、`load` 事件。

示例代码按 Saucer v8 文档中的事件名称书写；如果项目中 Saucer 版本或封装不同，按本地 API 名称调整即可。

```cpp
webview->on<saucer::webview::event::navigate>(
    [](const saucer::navigation& request) -> saucer::policy {
        std::println(
            "[saucer:navigate] url={} new_window={} redirect={} user={}",
            request->url().string(),
            request->new_window(),
            request->redirection(),
            request->user_initiated()
        );
        return saucer::policy::allow;
    }
);

webview->on<saucer::webview::event::navigated>(
    [](const saucer::uri& url) {
        std::println("[saucer:navigated] {}", url.string());
    }
);

webview->on<saucer::webview::event::request>(
    [](const saucer::uri& url) {
        std::println("[saucer:request] {}", url.string());
    }
);

webview->on<saucer::webview::event::load>(
    [](const saucer::state& state) {
        std::println(
            "[saucer:load] {}",
            state == saucer::state::started ? "started" : "finished"
        );
    }
);
```

需要关注的顺序是：

```text
[saucer:load] started
[page] boot href=/index.html 或 /index.html#/
[page] navigation.navigate-call /index.html#/auth
[saucer:navigate] 是否再次出现 /index.html#/auth
[saucer:load] 是否再次 started
[scheme] 是否再次请求 index.html
```

如果第二次跳转同时触发 `navigate` 和 `load started`，说明它被 backend 当成了一次文档导航。

如果只触发 `request` 或 custom scheme handler，需要继续看资源处理层是否把 hash URL 当成资源路径处理。

Saucer 的 `request` 事件在不同 backend 上行为不完全一致。它适合辅助观察，但不要单独用它下结论。

## 资源处理层探测

如果项目使用 Saucer embed 或 custom scheme，需要在 scheme handler 中打印收到的完整 URL、方法和 header。

```cpp
webview->handle_scheme("app", [](const saucer::scheme::request& req) {
    const auto url = req.url().string();

    std::println("[scheme] url={}", url);
    std::println("[scheme] method={}", req.method());

    for (const auto& [name, value] : req.headers()) {
        std::println("[scheme] header {}={}", name, value);
    }

    return saucer::scheme::response{
        .data = /* embedded file bytes */,
        .mime = "text/html",
        .status = 200,
    };
});
```

判读方式：

- 如果 handler 收到两次同一个 HTML 资源请求，第二次 URL 是 `/index.html#/auth`，说明 hash URL 进入了宿主资源层。
- 如果 handler 的资源查找代码把 `#/auth` 也纳入路径，需要在资源查找前确认只使用 URL 的 path 部分，不使用 fragment。
- 如果第二次没有进入 handler，但 Saucer `load` 重新开始，说明它更像是 WebView 导航生命周期问题。
- 如果宿主层完全没有第二次，只有页面层看到 Navigation API 事件，则问题不在资源请求。

WebView2 下还要确认 custom scheme URL 带有 authority，例如：

```text
app://root/index.html
```

避免使用：

```text
app://index.html
```

Saucer 文档提到 WebView2 下无 authority 的 scheme URL 可能把文件名当成 authority，导致意外行为。

## 页面层探测

在 `index.html` 中放一个早于 router 初始化的探针。这个代码只用于诊断，拿到日志后应删除。

```html
<script>
(() => {
  const log = (name, data = {}) => {
    console.log("[router-probe]", name, {
      t: performance.now(),
      href: location.href,
      navUrl: window.navigation?.currentEntry?.url,
      navKey: window.navigation?.currentEntry?.key,
      ...data,
    });
  };

  log("boot");

  window.addEventListener("pageshow", (event) => {
    log("pageshow", { persisted: event.persisted });
  });

  window.addEventListener("pagehide", (event) => {
    log("pagehide", { persisted: event.persisted });
  });

  window.addEventListener("beforeunload", () => {
    log("beforeunload");
  });

  window.navigation?.addEventListener("navigate", (event) => {
    log("navigation-event", {
      destination: event.destination?.url,
      type: event.navigationType,
      canIntercept: event.canIntercept,
      hashChange: event.hashChange,
      userInitiated: event.userInitiated,
    });
  });

  const rawNavigate = window.navigation?.navigate?.bind(window.navigation);
  if (rawNavigate) {
    window.navigation.navigate = (url, options) => {
      log("navigation.navigate-call", { url, options });
      return rawNavigate(url, options);
    };
  }

  setTimeout(() => {
    log("performance-navigation", {
      entries: performance.getEntriesByType("navigation").map((entry) => ({
        type: entry.type,
        name: entry.name,
        startTime: entry.startTime,
      })),
    });
  }, 0);
})();
</script>
```

重点看这些信号：

- `boot` 是否出现两次。出现两次说明页面脚本重新执行，基本可以认为发生了文档级重载。
- `beforeunload` / `pagehide` 是否在启动跳转时出现。出现说明当前 document 正在被卸载。
- `navigation-event.hashChange` 是否为 `true`。如果是 `true` 但宿主仍重新请求 HTML，需要怀疑 backend 对启动期 hash 导航的处理。
- `performance-navigation.entries` 是否多出新的 document navigation 记录。

## 对照实验

建议按下面矩阵逐项记录宿主日志和页面日志。

| 场景 | 期望观察点 |
| --- | --- |
| 初始 `/index.html#/auth` | 应只有一次 HTML 资源请求，作为基线 |
| 初始 `/index.html`，无守卫跳转 | 确认空 hash 场景本身是否稳定 |
| 初始 `/index.html#/`，无守卫跳转 | 确认 `#/` 本身是否触发额外请求 |
| 初始 `/index.html`，守卫跳转 `/auth` | 观察第二次请求是否紧跟 `navigation.navigate-call` |
| 初始 `/index.html#/`，守卫跳转 `/auth` | 观察第二次请求是否紧跟 `navigation.navigate-call` |
| 页面加载完成后手动 `navigation.navigate("/index.html#/auth")` | 对比启动期和稳定期行为 |
| 页面加载完成后手动 `history.replaceState(null, "", "/index.html#/auth")` | 判断 History API 是否触发宿主资源请求 |
| 页面加载完成后手动 `location.hash = "/auth"` | 判断普通 hash mutation 是否触发宿主资源请求 |

如果只有启动期守卫跳转触发第二次 HTML 请求，而稳定期的 `navigation.navigate(...)` 不触发，原因更可能是：

```text
首次 document 尚未完成 committed / load finished 时，
backend 把带 fragment 的 URL 更新升级成了 top-level document navigation。
```

如果稳定期所有 hash URL 修改都触发资源请求，原因更可能在 custom scheme / asset handler 或 backend 对 fragment 的处理。

## 后续决策边界

这份探测不改变 router 的设计：

- router 仍以 Navigation API 为核心，不重新引入 `popstate` / `hashchange` 兼容层。
- 不在没有证据前把启动重定向改成“只提交内部路由状态，不更新地址栏”。
- 不把 Saucer / WebView2 特定行为假设成所有浏览器的行为。

拿到日志后再决定修复方向：

- 如果确认是启动期 `navigation.navigate(...)` 被 backend 升级为文档导航，可以考虑只在启动阶段延迟 URL 更新，或等待 document 稳定后再执行守卫重定向。
- 如果确认是 scheme handler 路径解析错误，应修正资源层，只用 URL path 查找嵌入资源。
- 如果确认是无 authority scheme URL 引发的 WebView2 行为，应改用 `app://root/index.html` 这类稳定 URL 形态。
