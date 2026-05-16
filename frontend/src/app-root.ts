import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import type { RouterEventListener } from "@jsr/elelcode__lit-router";
import { saucerBridge } from "./bridge";
import "./components/toast-notification";
import "./pages/not-found-page";
import "./pages/error-page";
import { navigate, router } from "./router";
import {
  activeChatPubKey,
  adminStats,
  adminUsers,
  appendMessages,
  friends,
  identity,
  localIdentityAvailable,
  loadingRoute,
  outboxStatus,
  pushToast,
  removeFriend,
  resetSession,
  setFriendUnread,
  updateMessageById,
  upsertFriend,
} from "./store";
import type {
  AdminUpdate,
  CachedMessagesEvent,
  FriendRemovedEvent,
  FriendStatusEvent,
  MessageStatusEvent,
} from "./types";

const idleLockMs = 15 * 60 * 1000;

@customElement("app-root")
export class AppRoot extends SignalWatcher(LitElement) {
  private idleTimer: number | undefined;

  connectedCallback() {
    super.connectedCallback();
    router.addEventListener("route-loading-start", this.handleRouteLoadingStart);
    router.addEventListener("route-loading-end", this.handleRouteLoadingEnd);
    router.addEventListener("route-change", this.handleRouteChange);
    router.addEventListener("route-error", this.handleRouteError);
    router.addEventListener("route-not-found", this.handleRouteNotFound);
    window.addEventListener("saucer:friend-status", this.handleFriendStatus);
    window.addEventListener("saucer:system-broadcast", this.handleSystemBroadcast);
    window.addEventListener("saucer:force-offline", this.handleForceOffline);
    window.addEventListener("saucer:messages-pending", this.handleMessagesPending);
    window.addEventListener("saucer:admin-update", this.handleAdminUpdate);
    window.addEventListener("saucer:friend-removed", this.handleFriendRemoved);
    window.addEventListener("saucer:message-status", this.handleMessageStatus);
    window.addEventListener("saucer:cached-messages", this.handleCachedMessages);
    window.addEventListener("pointerdown", this.handleActivity, { passive: true });
    window.addEventListener("keydown", this.handleActivity);
    void this.refreshLocalIdentity();
    void this.refreshOutboxStatus();
    this.resetIdleTimer();
  }

  disconnectedCallback() {
    router.removeEventListener("route-loading-start", this.handleRouteLoadingStart);
    router.removeEventListener("route-loading-end", this.handleRouteLoadingEnd);
    router.removeEventListener("route-change", this.handleRouteChange);
    router.removeEventListener("route-error", this.handleRouteError);
    router.removeEventListener("route-not-found", this.handleRouteNotFound);
    window.removeEventListener("saucer:friend-status", this.handleFriendStatus);
    window.removeEventListener("saucer:system-broadcast", this.handleSystemBroadcast);
    window.removeEventListener("saucer:force-offline", this.handleForceOffline);
    window.removeEventListener("saucer:messages-pending", this.handleMessagesPending);
    window.removeEventListener("saucer:admin-update", this.handleAdminUpdate);
    window.removeEventListener("saucer:friend-removed", this.handleFriendRemoved);
    window.removeEventListener("saucer:message-status", this.handleMessageStatus);
    window.removeEventListener("saucer:cached-messages", this.handleCachedMessages);
    window.removeEventListener("pointerdown", this.handleActivity);
    window.removeEventListener("keydown", this.handleActivity);
    window.clearTimeout(this.idleTimer);
    super.disconnectedCallback();
  }

  private readonly handleRouteLoadingStart: RouterEventListener<"route-loading-start"> = () => {
    loadingRoute.set(true);
  };

  private readonly handleRouteLoadingEnd: RouterEventListener<"route-loading-end"> = (event) => {
    loadingRoute.set(event.detail.pending > 0);
  };

  private readonly handleRouteChange: RouterEventListener<"route-change"> = (event) => {
    const detail = event.detail;
    document.title = detail.leaf?.title ?? `ChatView ${detail.localPathname}`;
    this.resetIdleTimer();
  };

  private readonly handleRouteError: RouterEventListener<"route-error"> = (event) => {
    console.error("Route error:", event.detail);
  };

  private readonly handleRouteNotFound: RouterEventListener<"route-not-found"> = (event) => {
    console.warn("Route not found:", event.detail);
  };

  private readonly handleFriendStatus = (event: Event) => {
    const detail = (event as CustomEvent<FriendStatusEvent>).detail;
    upsertFriend({
      pubKey: detail.pubKey,
      alias: detail.alias ?? detail.pubKey.slice(0, 12),
      isOnline: detail.isOnline,
      unread: friends.get().find((friend) => friend.pubKey === detail.pubKey)?.unread ?? 0,
    });
  };

  private readonly handleFriendRemoved = (event: Event) => {
    const detail = (event as CustomEvent<FriendRemovedEvent>).detail;
    removeFriend(detail.pubKey);
  };

  private readonly handleSystemBroadcast = (event: Event) => {
    pushToast(String((event as CustomEvent<string>).detail), "info");
  };

  private readonly handleForceOffline = (event: Event) => {
    resetSession();
    pushToast(String((event as CustomEvent<string>).detail || "会话已下线"), "error");
    navigate({ name: "auth-unlock" });
  };

  private readonly handleMessagesPending = async (event: Event) => {
    const pubKey = String((event as CustomEvent<string>).detail);
    if (activeChatPubKey.get() === pubKey) {
      try {
        const page = await saucerBridge.getMessageHistory(pubKey, {
          direction: "newer",
          limit: 30,
        });
        appendMessages(pubKey, page.messages);
        setFriendUnread(pubKey, 0);
        void saucerBridge.markConversationRead(pubKey);
      } catch (error) {
        pushToast(error instanceof Error ? error.message : String(error), "error");
      }
      return;
    }
    const friend = friends.get().find((item) => item.pubKey === pubKey);
    setFriendUnread(pubKey, (friend?.unread ?? 0) + 1);
  };

  private readonly handleAdminUpdate = async () => {
    if (identity.get().role !== 1) return;
    const update: AdminUpdate = await saucerBridge.pollAdminEvents();
    adminUsers.set(update.users);
    adminStats.set(update.stats);
  };

  private readonly handleMessageStatus = (event: Event) => {
    const detail = (event as CustomEvent<MessageStatusEvent>).detail;
    const patch = {
      id: detail.serverMessageId ?? detail.clientMessageId,
      delivery: detail.delivery,
      error: detail.error,
      ...(detail.timestamp ? { timestamp: detail.timestamp } : {}),
    };
    updateMessageById(detail.clientMessageId, patch);
    void this.refreshOutboxStatus();
  };

  private readonly handleCachedMessages = (event: Event) => {
    const detail = (event as CustomEvent<CachedMessagesEvent>).detail;
    appendMessages(detail.pubKey, detail.messages);
  };

  private async refreshLocalIdentity() {
    try {
      localIdentityAvailable.set(await saucerBridge.hasLocalIdentity());
    } catch (error) {
      localIdentityAvailable.set(false);
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private async refreshOutboxStatus() {
    try {
      outboxStatus.set(await saucerBridge.getOutboxStatus());
    } catch {
      outboxStatus.set({ pending: 0, failed: 0 });
    }
  }

  private readonly handleActivity = () => {
    this.resetIdleTimer();
  };

  private resetIdleTimer() {
    window.clearTimeout(this.idleTimer);
    if (!identity.get().isLoggedIn) return;
    this.idleTimer = window.setTimeout(() => {
      void this.lockForIdle();
    }, idleLockMs);
  }

  private async lockForIdle() {
    if (!identity.get().isLoggedIn) return;
    try {
      await saucerBridge.lockSession();
    } catch (error) {
      console.warn("Failed to lock native session:", error);
    }
    resetSession();
    pushToast("会话因长时间无操作已锁定", "info");
    navigate({ name: "auth-unlock" });
  }

  render() {
    return html`
      <div class="loading ${loadingRoute.get() ? "is-active" : ""}"></div>
      <router-view .router=${router}>
        <not-found-page slot="404"></not-found-page>
        <error-page slot="error"></error-page>
      </router-view>
      <toast-notification></toast-notification>
    `;
  }

  static styles = css`
    :host {
      display: block;
      min-height: 100svh;
      width: 100%;
      color: #18202f;
      background: #f7f8fb;
    }

    .loading {
      position: fixed;
      inset: 0 0 auto 0;
      z-index: 10;
      height: 3px;
      transform: scaleX(0);
      transform-origin: left;
      background: #1f8a70;
      transition: transform 160ms ease;
    }

    .loading.is-active {
      transform: scaleX(1);
    }

  `;
}
