import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import { saucerBridge } from "../../bridge";
import { adminStats, adminStatus, adminUsers, pushToast } from "../../store";

@customElement("admin-dashboard")
export class AdminDashboard extends SignalWatcher(LitElement) {
  connectedCallback() {
    super.connectedCallback();
    void this.load();
  }

  private async load() {
    adminStatus.set({ ...adminStatus.get(), dashboardLoading: true, error: "" });
    try {
      const update = await saucerBridge.pollAdminEvents();
      adminUsers.set(update.users);
      adminStats.set(update.stats);
      adminStatus.set({ ...adminStatus.get(), dashboardLoading: false, error: "" });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      adminStatus.set({ ...adminStatus.get(), dashboardLoading: false, error: message });
      pushToast(message, "error");
    }
  }

  render() {
    const stats = adminStats.get();
    const status = adminStatus.get();
    return html`
      ${status.dashboardLoading ? html`<p class="state">正在加载看板</p>` : ""}
      ${status.error ? html`<p class="state error">${status.error}</p>` : ""}
      <section class="grid">
        <article>
          <span>在线人数</span>
          <strong>${stats.onlineUsers}</strong>
        </article>
        <article>
          <span>注册总数</span>
          <strong>${stats.totalUsers}</strong>
        </article>
        <article>
          <span>封禁用户</span>
          <strong>${stats.bannedUsers}</strong>
        </article>
      </section>
    `;
  }

  static styles = css`
    .grid {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 14px;
    }

    .state {
      margin: 0 0 14px;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
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
      display: grid;
      gap: 10px;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
      padding: 18px;
    }

    span {
      color: #657084;
      font-size: 13px;
    }

    strong {
      color: #18202f;
      font-size: 34px;
    }

    @media (max-width: 720px) {
      .grid {
        grid-template-columns: 1fr;
      }
    }
  `;
}
