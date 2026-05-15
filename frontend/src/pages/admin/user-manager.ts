import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import "@lit-labs/virtualizer";
import { saucerBridge } from "../../bridge";
import { adminStats, adminStatus, adminUsers, pushToast } from "../../store";
import type { UserInfo } from "../../types";

@customElement("user-manager")
export class UserManager extends SignalWatcher(LitElement) {
  connectedCallback() {
    super.connectedCallback();
    void this.load();
  }

  private async load() {
    adminStatus.set({ ...adminStatus.get(), usersLoading: true, error: "" });
    try {
      const update = await saucerBridge.pollAdminEvents();
      adminUsers.set(update.users);
      adminStats.set(update.stats);
      adminStatus.set({ ...adminStatus.get(), usersLoading: false, error: "" });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      adminStatus.set({ ...adminStatus.get(), usersLoading: false, error: message });
      pushToast(message, "error");
    }
  }

  private async setStatus(user: UserInfo) {
    try {
      await saucerBridge.adminSetUserStatus(user.pubKey, user.isBanned ? "active" : "banned");
      await this.load();
      pushToast(user.isBanned ? "用户已解封" : "用户已封禁", "success");
    } catch (error) {
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  render() {
    const users = adminUsers.get();
    const status = adminStatus.get();
    return html`
      <section class="manager">
        <header>
          <h2>用户列表</h2>
          <button type="button" ?disabled=${status.usersLoading} @click=${this.load}>
            ${status.usersLoading ? "刷新中" : "刷新"}
          </button>
        </header>
        ${status.error ? html`<p class="state error">${status.error}</p>` : ""}
        ${status.usersLoading && users.length === 0 ? html`<p class="state">正在加载用户</p>` : ""}
        ${!status.usersLoading && !status.error && users.length === 0 ? html`<p class="state">暂无用户</p>` : ""}
        ${users.length > 0
          ? html`
            <lit-virtualizer
              scroller
              .items=${users}
              .renderItem=${(user: UserInfo) => html`
                <article>
                  <span class="dot ${user.isOnline ? "online" : ""}"></span>
                  <code>${user.pubKey}</code>
                  <span class=${user.isBanned ? "banned" : "active"}>${user.isBanned ? "已封禁" : "正常"}</span>
                  <button type="button" @click=${() => this.setStatus(user)}>
                    ${user.isBanned ? "解封" : "封禁"}
                  </button>
                </article>
              `}
            ></lit-virtualizer>
          `
          : ""}
      </section>
    `;
  }

  static styles = css`
    .manager {
      box-sizing: border-box;
      display: grid;
      grid-template-rows: auto minmax(0, 1fr);
      min-height: 0;
      height: calc(100svh - 130px);
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
      overflow: hidden;
    }

    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      border-bottom: 1px solid #dce2ec;
      padding: 14px 16px;
    }

    h2 {
      margin: 0;
      color: #18202f;
      font-size: 18px;
    }

    lit-virtualizer {
      min-height: 0;
    }

    .state {
      margin: 16px;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #f7f8fb;
      color: #657084;
      padding: 12px;
      font-size: 13px;
    }

    .state.error {
      border-color: #ecc6c6;
      background: #fff3f3;
      color: #963131;
    }

    article {
      box-sizing: border-box;
      display: grid;
      grid-template-columns: 10px minmax(0, 1fr) 72px 76px;
      align-items: center;
      gap: 12px;
      border-bottom: 1px solid #edf0f5;
      padding: 12px 16px;
    }

    .dot {
      width: 10px;
      height: 10px;
      border-radius: 999px;
      background: #a9b3c3;
    }

    .dot.online {
      background: #1f8a70;
    }

    code {
      overflow: hidden;
      color: #273143;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .active,
    .banned {
      border-radius: 999px;
      padding: 4px 8px;
      text-align: center;
      font-size: 12px;
    }

    .active {
      background: #dff4ed;
      color: #176f5a;
    }

    .banned {
      background: #f8dfdf;
      color: #963131;
    }

    button {
      box-sizing: border-box;
      min-height: 32px;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      background: #ffffff;
      color: #273143;
      font-weight: 650;
    }

    button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
    }

    @media (max-width: 720px) {
      .manager {
        height: auto;
        min-height: 500px;
      }

      article {
        grid-template-columns: 10px minmax(0, 1fr);
      }

      article span:not(.dot),
      article button {
        grid-column: 2;
        justify-self: start;
      }
    }
  `;
}
