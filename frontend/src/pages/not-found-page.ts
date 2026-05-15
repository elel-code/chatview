import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { navigate } from "../router";

@customElement("not-found-page")
export class NotFoundPage extends LitElement {
  render() {
    return html`
      <main>
        <h1>页面不存在</h1>
        <p>当前地址没有匹配的前端路由。</p>
        <button type="button" @click=${() => navigate("/chat")}>返回聊天</button>
      </main>
    `;
  }

  static styles = css`
    main {
      display: grid;
      min-height: 100svh;
      place-content: center;
      gap: 12px;
      padding: 24px;
      text-align: center;
    }

    h1,
    p {
      margin: 0;
    }

    button {
      justify-self: center;
      border: 0;
      border-radius: 6px;
      background: #1f8a70;
      color: white;
      padding: 10px 14px;
    }
  `;
}
