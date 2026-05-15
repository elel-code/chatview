import { css, html, LitElement, nothing } from "lit";
import { customElement, query, state } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import "@lit-labs/virtualizer";
import type { LitVirtualizer } from "@lit-labs/virtualizer/LitVirtualizer.js";
import { saucerBridge } from "../../bridge";
import {
  activeChatPubKey,
  appendMessages,
  chatData,
  chatHistoryState,
  friends,
  identity,
  outboxStatus,
  prependMessages,
  pushToast,
  setChatHistoryState,
  updateMessage,
} from "../../store";
import type { ChatMessage } from "../../types";
import "./message-bubble";

type HistoryControlItem = { id: "__history-control" };
type ChatListItem = ChatMessage | HistoryControlItem;

@customElement("chat-panel")
export class ChatPanel extends SignalWatcher(LitElement) {
  @state()
  private draft = "";

  @state()
  private busy = false;

  @query("lit-virtualizer")
  private virtualizer?: LitVirtualizer<ChatMessage>;

  private stickToBottom = true;
  private pendingScrollRestore:
    | { scrollTop: number; scrollHeight: number }
    | undefined;
  private topLoadArmed = true;
  private autoLoadPubKey = "";

  protected updated() {
    if (this.pendingScrollRestore) {
      const restore = this.pendingScrollRestore;
      this.pendingScrollRestore = undefined;
      requestAnimationFrame(() => {
        const scroller = this.scroller;
        if (!scroller) return;
        scroller.scrollTop = restore.scrollTop + (scroller.scrollHeight - restore.scrollHeight);
      });
      return;
    }

    const pubKey = activeChatPubKey.get();
    const messages = chatData.get()[pubKey] ?? [];
    if (messages.length > 0 && this.stickToBottom) {
      const history = chatHistoryState.get()[pubKey];
      const offset = this.hasHistoryControl(messages, history) ? 1 : 0;
      this.virtualizer?.scrollToIndex(messages.length - 1 + offset, "end");
    }
  }

  protected willUpdate() {
    const pubKey = activeChatPubKey.get();
    if (pubKey) {
      void this.ensureHistory(pubKey);
    }
  }

  private async ensureHistory(pubKey: string) {
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
      await saucerBridge.markConversationRead(pubKey);
    } catch (error) {
      setChatHistoryState(pubKey, { loading: false });
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private async loadMoreHistory() {
    const pubKey = activeChatPubKey.get();
    if (!pubKey) return;
    const state = chatHistoryState.get()[pubKey];
    if (state?.loading || !state?.hasMore) return;
    const scroller = this.scroller;
    this.pendingScrollRestore = scroller
      ? { scrollTop: scroller.scrollTop, scrollHeight: scroller.scrollHeight }
      : undefined;
    setChatHistoryState(pubKey, { loading: true });
    try {
      const page = await saucerBridge.getMessageHistory(pubKey, {
        cursor: state.nextCursor,
        direction: "older",
        limit: 30,
      });
      prependMessages(pubKey, page.messages);
      setChatHistoryState(pubKey, {
        loaded: true,
        loading: false,
        nextCursor: page.nextCursor,
        hasMore: page.hasMore,
      });
    } catch (error) {
      setChatHistoryState(pubKey, { loading: false });
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private hasHistoryControl(
    messages: ChatMessage[],
    history: ReturnType<typeof chatHistoryState.get>[string] | undefined,
  ) {
    return Boolean(history?.loading || history?.hasMore || (history?.loaded && messages.length > 0));
  }

  private getListItems(
    messages: ChatMessage[],
    history: ReturnType<typeof chatHistoryState.get>[string] | undefined,
  ): ChatListItem[] {
    return this.hasHistoryControl(messages, history)
      ? [{ id: "__history-control" }, ...messages]
      : messages;
  }

  private isHistoryControl(item: ChatListItem): item is HistoryControlItem {
    return item.id === "__history-control";
  }

  private renderHistoryControl(
    messages: ChatMessage[],
    history: ReturnType<typeof chatHistoryState.get>[string] | undefined,
  ) {
    if (history?.loading) {
      return html`
        <div class="history-row">
          <div class="history-control loading" aria-live="polite">
            <span class="spinner"></span>
            <span>正在同步历史</span>
            ${messages.length === 0 ? html`
              <div class="history-skeleton">
                <i></i>
                <i></i>
                <i></i>
              </div>
            ` : nothing}
          </div>
        </div>
      `;
    }

    if (history?.hasMore) {
      return html`
        <div class="history-row">
          <button class="history-control action" type="button" @click=${this.loadMoreHistory}>
            <span>加载更早消息</span>
          </button>
        </div>
      `;
    }

    return messages.length > 0
      ? html`<div class="history-row"><div class="history-control done">已到最早消息</div></div>`
      : nothing;
  }

  private async send(event: SubmitEvent) {
    event.preventDefault();
    const text = this.draft.trim();
    const receiver = activeChatPubKey.get();
    if (!text || !receiver) return;

    const localId = crypto.randomUUID();
    this.busy = true;
    this.draft = "";
    this.stickToBottom = true;
    const message: ChatMessage = {
      id: localId,
      sender: identity.get().publicKey || "me",
      text,
      timestamp: new Date().toISOString(),
      delivery: "pending",
    };
    appendMessages(receiver, [message]);

    try {
      const result = await saucerBridge.sendMessage(receiver, text, localId);
      updateMessage(receiver, localId, {
        id: result?.messageId ?? localId,
        timestamp: result?.timestamp ?? new Date().toISOString(),
        delivery: "sent",
        error: undefined,
      });
    } catch (error) {
      const messageError = error instanceof Error ? error.message : String(error);
      updateMessage(receiver, localId, { delivery: "failed", error: messageError });
      outboxStatus.set(await saucerBridge.getOutboxStatus());
    } finally {
      this.busy = false;
    }
  }

  private async retry(message: ChatMessage) {
    const receiver = activeChatPubKey.get();
    if (!receiver) return;
    updateMessage(receiver, message.id, { delivery: "pending", error: undefined });
    try {
      await saucerBridge.retryOutboxMessage(message.id);
      pushToast("已请求重试，等待发件箱状态更新", "info");
    } catch (error) {
      updateMessage(receiver, message.id, {
        delivery: "failed",
        error: error instanceof Error ? error.message : String(error),
      });
      pushToast("消息重试失败", "error");
    }
  }

  render() {
    const pubKey = activeChatPubKey.get();
    const friend = friends.get().find((item) => item.pubKey === pubKey);
    const messages = chatData.get()[pubKey] ?? [];
    const history = chatHistoryState.get()[pubKey];
    const outbox = outboxStatus.get();
    const items = this.getListItems(messages, history);

    if (!pubKey) {
      return html`<section class="empty">选择一个好友开始聊天</section>`;
    }

    return html`
      <section class="panel">
        <header>
          <div>
            <h2>${friend?.alias ?? pubKey}</h2>
            <span>${pubKey}</span>
          </div>
          <i class=${friend?.isOnline ? "online" : ""}>${friend?.isOnline ? "在线" : "离线"}</i>
        </header>
        ${outbox.pending || outbox.failed
          ? html`<div class="outbox">
            ${outbox.pending ? html`<span>${outbox.pending} 条待发送</span>` : ""}
            ${outbox.failed ? html`<span class="failed">${outbox.failed} 条发送失败</span>` : ""}
          </div>`
          : nothing}
        <lit-virtualizer
          scroller
          @scroll=${this.handleScroll}
          .items=${items}
          .renderItem=${(item: ChatListItem) => this.isHistoryControl(item)
            ? this.renderHistoryControl(messages, history)
            : html`
              <message-bubble .msg=${item} .own=${item.sender === (identity.get().publicKey || "me")}></message-bubble>
          `}
          @message-retry=${(event: CustomEvent<ChatMessage>) => this.retry(event.detail)}
        ></lit-virtualizer>
        ${messages.length === 0 && !history?.loading ? html`<p class="placeholder">还没有历史消息</p>` : nothing}
        <form @submit=${this.send}>
          <textarea
            rows="2"
            placeholder="输入消息，Enter 发送"
            .value=${this.draft}
            @input=${(event: InputEvent) => this.draft = (event.target as HTMLTextAreaElement).value}
            @keydown=${(event: KeyboardEvent) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                this.requestSubmit();
              }
            }}
          ></textarea>
          <button type="submit" ?disabled=${this.busy || !this.draft.trim()}>发送</button>
        </form>
      </section>
    `;
  }

  private requestSubmit() {
    this.renderRoot.querySelector("form")?.requestSubmit();
  }

  private get scroller() {
    return this.virtualizer as unknown as HTMLElement | undefined;
  }

  private readonly handleScroll = () => {
    const scroller = this.scroller;
    if (!scroller) return;
    const pubKey = activeChatPubKey.get();
    if (pubKey !== this.autoLoadPubKey) {
      this.autoLoadPubKey = pubKey;
      this.topLoadArmed = true;
    }

    this.stickToBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 96;
    if (scroller.scrollTop > 180) {
      this.topLoadArmed = true;
    }

    const history = chatHistoryState.get()[pubKey];
    const messages = chatData.get()[pubKey] ?? [];
    if (scroller.scrollTop < 96 && this.topLoadArmed && history?.hasMore && !history.loading && messages.length > 0) {
      this.topLoadArmed = false;
      void this.loadMoreHistory();
    }
  };

  static styles = css`
    .panel {
      position: relative;
      display: grid;
      grid-template-rows: auto minmax(0, 1fr) auto;
      height: 100svh;
      min-width: 0;
      min-height: 0;
      background: #f7f8fb;
    }

    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      border-bottom: 1px solid #dce2ec;
      background: #ffffff;
      padding: 16px 20px;
    }

    header > div {
      min-width: 0;
    }

    h2 {
      overflow: hidden;
      margin: 0;
      color: #18202f;
      font-size: 18px;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    header span {
      display: block;
      overflow: hidden;
      max-width: 52vw;
      color: #657084;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    i {
      border-radius: 999px;
      background: #e7ebf2;
      color: #657084;
      padding: 5px 10px;
      font-size: 12px;
      font-style: normal;
    }

    i.online {
      background: #dff4ed;
      color: #1f8a70;
    }

    lit-virtualizer {
      min-height: 0;
      padding: 12px 0;
    }

    .outbox {
      display: flex;
      gap: 10px;
      border-bottom: 1px solid #dce2ec;
      background: #fff8e8;
      color: #7b5b13;
      padding: 8px 20px;
      font-size: 12px;
    }

    .outbox .failed {
      color: #963131;
    }

    form {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 10px;
      border-top: 1px solid #dce2ec;
      background: #ffffff;
      padding: 14px;
    }

    textarea {
      box-sizing: border-box;
      min-width: 0;
      border: 1px solid #cfd6e2;
      border-radius: 8px;
      padding: 10px 12px;
      resize: none;
    }

    button {
      min-width: 82px;
      border: 0;
      border-radius: 8px;
      background: #1f8a70;
      color: #ffffff;
      font-weight: 700;
    }

    button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
    }

    .history-row {
      display: flex;
      justify-content: center;
      box-sizing: border-box;
      width: 100%;
      padding: 4px 16px 14px;
    }

    .history-control {
      box-sizing: border-box;
      width: fit-content;
      max-width: min(420px, calc(100% - 32px));
      min-height: 34px;
      margin: 0;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: rgba(255, 255, 255, 0.94);
      color: #657084;
      box-shadow: 0 8px 24px rgba(24, 32, 47, 0.06);
      padding: 8px 12px;
      font-size: 12px;
      line-height: 1.35;
    }

    .history-control.action {
      display: flex;
      align-items: center;
      justify-content: center;
      min-width: 146px;
      color: #273143;
      font-weight: 650;
      cursor: pointer;
    }

    .history-control.action:hover {
      border-color: #9fb0c5;
      background: #ffffff;
    }

    .history-control.loading {
      display: grid;
      grid-template-columns: auto auto;
      align-items: center;
      justify-content: center;
      gap: 8px;
      min-width: 168px;
    }

    .history-control.done {
      border-color: transparent;
      background: transparent;
      box-shadow: none;
      color: #8a94a6;
      padding-block: 4px;
    }

    .spinner {
      width: 14px;
      height: 14px;
      border: 2px solid #cfd6e2;
      border-top-color: #1f8a70;
      border-radius: 999px;
      animation: spin 800ms linear infinite;
    }

    .history-skeleton {
      grid-column: 1 / -1;
      display: grid;
      gap: 8px;
      width: min(320px, 68vw);
      margin-top: 6px;
    }

    .history-skeleton i {
      display: block;
      height: 10px;
      border-radius: 999px;
      background: linear-gradient(90deg, #eef2f6 0%, #dfe6ef 45%, #eef2f6 90%);
      background-size: 220% 100%;
      animation: shimmer 1100ms ease-in-out infinite;
    }

    .history-skeleton i:nth-child(2) {
      width: 78%;
    }

    .history-skeleton i:nth-child(3) {
      width: 56%;
    }

    @keyframes spin {
      to {
        transform: rotate(360deg);
      }
    }

    @keyframes shimmer {
      from {
        background-position: 120% 0;
      }
      to {
        background-position: -120% 0;
      }
    }

    .empty,
    .placeholder {
      display: grid;
      place-items: center;
      color: #657084;
    }

    .empty {
      min-height: 100svh;
    }

    .placeholder {
      position: absolute;
      inset: 74px 0 80px 0;
      pointer-events: none;
    }

    @media (max-width: 900px) {
      .panel {
        height: 72svh;
      }
    }
  `;
}
