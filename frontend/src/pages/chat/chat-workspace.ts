import { css, html, LitElement } from "lit";
import { customElement, property } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import type { RouteContext } from "@jsr/elelcode__lit-router";
import { saucerBridge } from "../../bridge";
import {
  chatData,
  chatHistoryState,
  friendListStatus,
  prependMessages,
  pushToast,
  setActiveChat,
  setChatHistoryState,
  setFriends,
} from "../../store";
import "./friend-list";
import "./chat-panel";

@customElement("chat-workspace")
export class ChatWorkspace extends SignalWatcher(LitElement) {
  @property({ attribute: false })
  accessor routeContext: RouteContext | undefined;

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener("chatview:reload-friends", this.handleFriendsReload);
    void this.loadFriends();
  }

  disconnectedCallback() {
    window.removeEventListener("chatview:reload-friends", this.handleFriendsReload);
    super.disconnectedCallback();
  }

  private readonly handleFriendsReload = () => {
    void this.loadFriends();
  };

  private async loadFriends() {
    friendListStatus.set({ loading: true, error: "" });
    try {
      const nextFriends = await saucerBridge.listFriends();
      setFriends(nextFriends);
      if (nextFriends.length === 0) {
        setActiveChat("");
      }
      friendListStatus.set({ loading: false, error: "" });

      // SQLite 缓存预加载: 后台拉取每个好友的最近消息
      // C++ 层对 direction=older 且 cursor 为空走 SQLite 快速返回
      for (const friend of nextFriends) {
        void this.preloadHistory(friend.pubKey);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      friendListStatus.set({ loading: false, error: message });
      pushToast(message, "error");
    }
  }

  private async preloadHistory(pubKey: string) {
    if ((chatData.get()[pubKey]?.length ?? 0) > 0) return;
    const state = chatHistoryState.get()[pubKey];
    if (state?.loaded || state?.loading) return;
    setChatHistoryState(pubKey, { loading: true });
    try {
      const page = await saucerBridge.getMessageHistory(pubKey, { limit: 30 });
      prependMessages(pubKey, page.messages);
      setChatHistoryState(pubKey, {
        loaded: true,
        loading: false,
        nextCursor: page.nextCursor,
        hasMore: page.hasMore,
      });
    } catch {
      setChatHistoryState(pubKey, { loading: false });
      // 预加载失败不弹 toast，用户点开好友时 ensureHistory 会重试
    }
  }

  render() {
    return html`
      <section class="workspace">
        <friend-list></friend-list>
        <chat-panel></chat-panel>
      </section>
    `;
  }

  static styles = css`
    .workspace {
      display: grid;
      grid-template-columns: 320px minmax(0, 1fr);
      height: 100svh;
      min-height: 0;
    }

    @media (max-width: 900px) {
      .workspace {
        grid-template-columns: 1fr;
        height: auto;
      }
    }
  `;
}
