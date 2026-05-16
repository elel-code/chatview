import { css, html, LitElement } from "lit";
import { customElement, property } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import type { RouteContext } from "@jsr/elelcode__lit-router";
import { navigate } from "../../router";

@customElement("admin-shell")
export class AdminShell extends SignalWatcher(LitElement) {
  @property({ attribute: false })
  accessor routeContext: RouteContext | undefined;

  render() {
    const routeName = this.routeContext?.detail.leaf?.name ?? "admin-dashboard";
    return html`
      <section class="shell">
        <header>
          <div>
            <h1>管理员控制台</h1>
            <span>用户状态、全服广播和运行概览</span>
          </div>
          <nav>
            <button class=${routeName === "admin-dashboard" ? "active" : ""} @click=${() => navigate({ name: "admin-dashboard" })}>看板</button>
            <button class=${routeName === "admin-users" ? "active" : ""} @click=${() => navigate({ name: "admin-users" })}>用户</button>
            <button class=${routeName === "admin-broadcast" ? "active" : ""} @click=${() => navigate({ name: "admin-broadcast" })}>广播</button>
          </nav>
        </header>
        <slot name="route-child"></slot>
      </section>
    `;
  }

  static styles = css`
    .shell {
      box-sizing: border-box;
      display: grid;
      grid-template-rows: auto minmax(0, 1fr);
      min-height: 100svh;
      padding: 22px;
      gap: 18px;
    }

    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      border-bottom: 1px solid #dce2ec;
      padding-bottom: 16px;
    }

    h1 {
      margin: 0;
      color: #18202f;
      font-size: 24px;
    }

    span {
      color: #657084;
      font-size: 13px;
    }

    nav {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
    }

    button {
      box-sizing: border-box;
      min-height: 36px;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      background: #ffffff;
      color: #273143;
      padding: 0 14px;
      font-weight: 650;
    }

    button.active {
      border-color: #1f8a70;
      background: #e7f7f2;
      color: #176f5a;
    }

    @media (max-width: 720px) {
      .shell {
        padding: 16px;
      }

      header {
        align-items: stretch;
        flex-direction: column;
      }
    }
  `;
}
