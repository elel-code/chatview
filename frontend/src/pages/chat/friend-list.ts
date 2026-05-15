import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import { saucerBridge } from "../../bridge";
import {
  activeChatPubKey,
  appendMessages,
  chatHistoryState,
  friendListStatus,
  friends,
  prependMessages,
  pushToast,
  setActiveChat,
  setChatHistoryState,
  setFriendUnread,
} from "../../store";

@customElement("friend-list")
export class FriendList extends SignalWatcher(LitElement) {
  private async select(pubKey: string) {
    setActiveChat(pubKey);
    setFriendUnread(pubKey, 0);
    try {
      await this.ensureHistory(pubKey);
      const page = await saucerBridge.getMessageHistory(pubKey, {
        direction: "newer",
        limit: 30,
      });
      if (page.messages.length > 0) {
        appendMessages(pubKey, page.messages);
      }
      await saucerBridge.markConversationRead(pubKey);
    } catch (error) {
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private async ensureHistory(pubKey: string) {
    const state = chatHistoryState.get()[pubKey];
    if (state?.loaded || state?.loading) return;
    setChatHistoryState(pubKey, { loading: true });
    const page = await saucerBridge.getMessageHistory(pubKey, { limit: 30 });
    prependMessages(pubKey, page.messages);
    setChatHistoryState(pubKey, {
      loaded: true,
      loading: false,
      nextCursor: page.nextCursor,
      hasMore: page.hasMore,
    });
  }

  render() {
    const active = activeChatPubKey.get();
    const status = friendListStatus.get();
    const items = friends.get();
    return html`
      <aside>
        <header>
          <h2>好友</h2>
          <button type="button" class="refresh" @click=${this.reload}>刷新</button>
        </header>
        ${status.loading ? html`<p class="state">正在加载好友</p>` : ""}
        ${status.error
          ? html`
            <div class="state error">
              <span>${status.error}</span>
              <button type="button" @click=${this.reload}>重试</button>
            </div>
          `
          : ""}
        ${!status.loading && !status.error && items.length === 0
          ? html`<p class="state">暂无好友，请从左侧添加公钥。</p>`
          : ""}
        <div class="list">
          ${items.map(
            (friend) => html`
              <button
                type="button"
                class=${friend.pubKey === active ? "active" : ""}
                @click=${() => this.select(friend.pubKey)}
              >
                <span class="status ${friend.isOnline ? "online" : ""}"></span>
                <span class="meta">
                  <strong>${friend.alias}</strong>
                  <small>${friend.pubKey}</small>
                </span>
                ${friend.unread ? html`<b>${friend.unread}</b>` : ""}
              </button>
            `,
          )}
        </div>
      </aside>
    `;
  }

  private reload() {
    window.dispatchEvent(new CustomEvent("chatview:reload-friends"));
  }

  static styles = css`
    aside {
      min-width: 0;
      border-right: 1px solid #dce2ec;
      background: #ffffff;
      padding: 18px;
      overflow: auto;
    }

    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 14px;
    }

    h2 {
      margin: 0;
      color: #18202f;
      font-size: 18px;
    }

    .refresh,
    .state button {
      box-sizing: border-box;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      background: #ffffff;
      color: #273143;
      padding: 6px 10px;
      font-size: 12px;
      font-weight: 650;
    }

    .state {
      display: grid;
      gap: 8px;
      margin: 0 0 12px;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #f7f8fb;
      color: #657084;
      padding: 12px;
      font-size: 13px;
      line-height: 1.45;
    }

    .state.error {
      border-color: #ecc6c6;
      background: #fff3f3;
      color: #963131;
    }

    .list {
      display: grid;
      gap: 8px;
    }

    button {
      box-sizing: border-box;
      display: grid;
      grid-template-columns: 10px minmax(0, 1fr) auto;
      align-items: center;
      gap: 10px;
      width: 100%;
      border: 1px solid transparent;
      border-radius: 8px;
      background: #f7f8fb;
      padding: 12px;
      text-align: left;
    }

    button.active {
      border-color: #1f8a70;
      background: #eefaf6;
    }

    .status {
      width: 10px;
      height: 10px;
      border-radius: 999px;
      background: #a9b3c3;
    }

    .status.online {
      background: #1f8a70;
    }

    .meta {
      display: grid;
      min-width: 0;
      gap: 2px;
    }

    strong,
    small {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    strong {
      color: #18202f;
    }

    small {
      color: #657084;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
    }

    b {
      min-width: 22px;
      border-radius: 999px;
      background: #bf3b3b;
      color: #ffffff;
      padding: 2px 7px;
      text-align: center;
      font-size: 12px;
    }

    @media (max-width: 900px) {
      aside {
        border-right: 0;
        border-bottom: 1px solid #dce2ec;
        max-height: 280px;
      }
    }
  `;
}
