import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";

@customElement("error-page")
export class ErrorPage extends LitElement {
  render() {
    return html`
      <main>
        <h1>路由加载失败</h1>
        <p>请检查懒加载模块或路由守卫返回值。</p>
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
  `;
}
