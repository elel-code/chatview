import { css, html, LitElement } from "lit";
import { customElement, state } from "lit/decorators.js";
import { saucerBridge } from "../../bridge";
import { pushToast } from "../../store";

@customElement("broadcast-panel")
export class BroadcastPanel extends LitElement {
  @state()
  private text = "";

  @state()
  private busy = false;

  private async submit(event: SubmitEvent) {
    event.preventDefault();
    const text = this.text.trim();
    if (!text || text.length > 500) return;
    this.busy = true;
    try {
      await saucerBridge.adminBroadcast(text);
      this.text = "";
      pushToast("广播已发送", "success");
    } catch (error) {
      pushToast(error instanceof Error ? error.message : String(error), "error");
    } finally {
      this.busy = false;
    }
  }

  render() {
    const text = this.text.trim();
    const tooLong = text.length > 500;
    const nearLimit = this.text.length >= 450;
    return html`
      <form @submit=${this.submit}>
        <label>
          全服广播内容
          <textarea
            rows="8"
            maxlength="500"
            .value=${this.text}
            @input=${(event: InputEvent) => this.text = (event.target as HTMLTextAreaElement).value}
          ></textarea>
        </label>
        <footer>
          <span class=${nearLimit || tooLong ? "limit warn" : "limit"}>${this.text.length}/500</span>
          <button type="submit" ?disabled=${this.busy || !text || tooLong}>
            ${this.busy ? "发送中" : "发送广播"}
          </button>
        </footer>
        ${tooLong ? html`<p class="error">广播内容不能超过 500 个字符。</p>` : ""}
      </form>
    `;
  }

  static styles = css`
    form {
      box-sizing: border-box;
      display: grid;
      gap: 14px;
      width: 100%;
      max-width: 720px;
      border: 1px solid #dce2ec;
      border-radius: 8px;
      background: #ffffff;
      padding: 18px;
    }

    label {
      display: grid;
      gap: 10px;
      color: #536078;
      font-size: 13px;
      font-weight: 650;
    }

    textarea {
      box-sizing: border-box;
      width: 100%;
      border: 1px solid #cfd6e2;
      border-radius: 8px;
      padding: 12px;
      resize: vertical;
      overflow-wrap: anywhere;
    }

    footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .limit {
      color: #657084;
      font-size: 12px;
    }

    .limit.warn {
      color: #a56a00;
      font-weight: 700;
    }

    .error {
      margin: -4px 0 0;
      color: #bf3b3b;
      font-size: 13px;
    }

    button {
      box-sizing: border-box;
      min-height: 40px;
      border: 0;
      border-radius: 6px;
      background: #1f8a70;
      color: #ffffff;
      padding: 0 16px;
      font-weight: 700;
    }

    button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
    }

    @media (max-width: 520px) {
      footer {
        align-items: stretch;
        flex-direction: column;
      }

      button {
        width: 100%;
      }
    }
  `;
}
