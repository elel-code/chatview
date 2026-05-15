import { css, html, LitElement } from "lit";
import { customElement, property } from "lit/decorators.js";
import type { RouteContext } from "@jsr/elelcode__lit-router";
import { navigate } from "../../router";

@customElement("auth-view")
export class AuthView extends LitElement {
  @property({ attribute: false })
  accessor routeContext: RouteContext | undefined;

  render() {
    const path = this.routeContext?.detail.localPathname ?? "/auth";
    return html`
      <main>
        <section class="panel">
          <div class="brand">
            <span>ChatView</span>
            <strong>去中心化身份聊天</strong>
          </div>
          <nav>
            <button class=${path === "/auth" ? "active" : ""} @click=${() => navigate("/auth")}>解锁</button>
            <button class=${path === "/auth/create" ? "active" : ""} @click=${() => navigate("/auth/create")}>创建</button>
            <button class=${path === "/auth/import" ? "active" : ""} @click=${() => navigate("/auth/import")}>导入</button>
          </nav>
          <slot name="route-child"></slot>
        </section>
      </main>
    `;
  }

  static styles = css`
    main {
      display: grid;
      min-height: 100svh;
      place-items: center;
      padding: 24px;
      background:
        linear-gradient(115deg, rgba(31, 138, 112, 0.12), transparent 42%),
        linear-gradient(280deg, rgba(65, 105, 225, 0.1), transparent 38%),
        #f7f8fb;
    }

    .panel {
      display: grid;
      gap: 20px;
      width: min(460px, 100%);
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
      box-shadow: 0 22px 60px rgba(31, 43, 68, 0.13);
      padding: 24px;
    }

    .brand {
      display: grid;
      gap: 4px;
    }

    .brand span {
      color: #1f8a70;
      font-weight: 700;
      letter-spacing: 0;
    }

    .brand strong {
      color: #18202f;
      font-size: 22px;
      font-weight: 700;
    }

    nav {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 6px;
      border-radius: 8px;
      background: #eef2f7;
      padding: 4px;
    }

    button {
      min-height: 36px;
      border: 0;
      border-radius: 6px;
      background: transparent;
      color: #536078;
      font-size: 14px;
    }

    button.active {
      background: #ffffff;
      color: #18202f;
      box-shadow: 0 1px 5px rgba(31, 43, 68, 0.12);
    }
  `;
}
