import { css, html, LitElement, nothing } from "lit";
import { customElement, query, state } from "lit/decorators.js";
import { saucerBridge } from "../bridge";
import { identity, pushToast } from "../store";

@customElement("identity-card")
export class IdentityCard extends LitElement {
  @state()
  private exportPin = "";

  @state()
  private privateKey = "";

  @state()
  private error = "";

  @query("dialog")
  private exportDialog?: HTMLDialogElement;

  private async copyPublicKey() {
    await navigator.clipboard.writeText(identity.get().publicKey);
    pushToast("公钥已复制", "success");
  }

  private async exportPrivateKey() {
    this.error = "";
    try {
      this.privateKey = await saucerBridge.exportPrivateKey(this.exportPin);
      pushToast("私钥已解锁，请确认已在安全环境中查看", "info");
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  private openExportDialog() {
    this.error = "";
    this.exportPin = "";
    this.privateKey = "";
    this.exportDialog?.showModal();
  }

  private closeExportDialog() {
    this.exportDialog?.close();
  }

  private handleExportDialogClosed() {
    this.exportPin = "";
    this.privateKey = "";
    this.error = "";
  }

  render() {
    const ident = identity.get();
    return html`
      <section>
        <div class="label">本机身份</div>
        <div class="key" title=${ident.publicKey}>${ident.publicKey}</div>
        <div class="actions">
          <button type="button" @click=${this.copyPublicKey}>复制</button>
          <button type="button" @click=${this.openExportDialog}>导出</button>
        </div>
        <dialog @close=${this.handleExportDialogClosed}>
          <form method="dialog">
            <header>
              <strong>导出私钥</strong>
              <button type="button" class="icon" @click=${this.closeExportDialog}>×</button>
            </header>
            <label>
              本地 PIN
              <input
                type="password"
                autocomplete="current-password"
                placeholder="输入 PIN"
                .value=${this.exportPin}
                @input=${(event: InputEvent) => this.exportPin = (event.target as HTMLInputElement).value}
              />
            </label>
            <button type="button" class="primary" @click=${this.exportPrivateKey}>验证并显示</button>
            ${this.error ? html`<p class="error">${this.error}</p>` : nothing}
            ${this.privateKey ? html`<pre>${this.privateKey}</pre>` : nothing}
          </form>
        </dialog>
      </section>
    `;
  }

  static styles = css`
    section {
      display: grid;
      gap: 10px;
      padding: 14px;
      border: 1px solid #d9dee8;
      border-radius: 8px;
      background: #ffffff;
    }

    .label {
      color: #657084;
      font-size: 12px;
    }

    .key {
      overflow: hidden;
      color: #18202f;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 13px;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .actions {
      display: flex;
      gap: 8px;
    }

    button {
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      background: #f7f8fb;
      color: #273143;
      padding: 7px 10px;
      font-size: 13px;
    }

    dialog {
      width: min(460px, calc(100vw - 32px));
      border: 1px solid #dce2ec;
      border-radius: 8px;
      box-shadow: 0 22px 60px rgba(31, 43, 68, 0.22);
      padding: 0;
    }

    dialog::backdrop {
      background: rgba(16, 24, 36, 0.38);
    }

    form {
      display: grid;
      gap: 12px;
      padding: 18px;
    }

    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    header strong {
      color: #18202f;
      font-size: 16px;
    }

    label {
      display: grid;
      gap: 8px;
      color: #536078;
      font-size: 13px;
      font-weight: 650;
    }

    input {
      box-sizing: border-box;
      min-width: 0;
      width: 100%;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      padding: 9px 10px;
    }

    .primary {
      background: #1f8a70;
      color: #ffffff;
      border-color: #1f8a70;
      min-height: 38px;
    }

    .icon {
      width: 30px;
      height: 30px;
      padding: 0;
      font-size: 18px;
      line-height: 1;
    }

    pre {
      overflow: auto;
      max-height: 110px;
      margin: 0;
      border-radius: 6px;
      background: #101824;
      color: #eef5ff;
      padding: 10px;
      font-size: 12px;
      white-space: pre-wrap;
      word-break: break-all;
    }

    .error {
      margin: 0;
      color: #bf3b3b;
      font-size: 12px;
    }
  `;
}
