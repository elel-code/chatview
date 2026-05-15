import { css, html, LitElement } from "lit";
import { customElement, property } from "lit/decorators.js";
import type { ChatMessage } from "../../types";

@customElement("message-bubble")
export class MessageBubble extends LitElement {
  @property({ attribute: false })
  accessor msg: ChatMessage | undefined;

  @property({ type: Boolean })
  accessor own = false;

  render() {
    if (!this.msg) return html``;
    return html`
      <article class=${this.own ? "own" : ""}>
        <div>
          <p>${this.msg.text}</p>
          <footer>
            <time>${new Date(this.msg.timestamp).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}</time>
            ${this.own ? this.renderDelivery() : ""}
          </footer>
        </div>
      </article>
    `;
  }

  private renderDelivery() {
    if (!this.msg?.delivery || this.msg.delivery === "incoming") return "";
    if (this.msg.delivery === "failed") {
      return html`
        <button
          type="button"
          title=${this.msg.error ?? "发送失败"}
          @click=${() => this.dispatchEvent(new CustomEvent("message-retry", {
            detail: this.msg,
            bubbles: true,
            composed: true,
          }))}
        >
          重试
        </button>
      `;
    }
    return html`<span>${this.msg.delivery === "pending" ? "发送中" : "已发送"}</span>`;
  }

  static styles = css`
    article {
      display: flex;
      padding: 5px 12px;
    }

    article.own {
      justify-content: flex-end;
    }

    div {
      min-width: 0;
      max-width: min(620px, 74%);
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
      padding: 10px 12px;
    }

    .own div {
      border-color: #1f8a70;
      background: #e7f7f2;
    }

    p {
      margin: 0;
      color: #18202f;
      line-height: 1.48;
      overflow-wrap: anywhere;
      word-break: break-word;
    }

    footer {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 5px;
      flex-wrap: wrap;
    }

    time,
    span,
    button {
      color: #657084;
      font-size: 11px;
    }

    button {
      border: 0;
      background: transparent;
      color: #bf3b3b;
      padding: 0;
      text-decoration: underline;
    }
  `;
}
