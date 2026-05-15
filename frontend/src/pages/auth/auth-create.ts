import { css, html, LitElement, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { saucerBridge } from "../../bridge";
import { completeLogin } from "../../router";
import { localIdentityAvailable, pushToast } from "../../store";

@customElement("auth-create")
export class AuthCreate extends LitElement {
  @state()
  private pin = "";

  @state()
  private publicKey = "";

  @state()
  private privateKey = "";

  @state()
  private backedUp = false;

  @state()
  private error = "";

  @state()
  private busy = false;

  private async create(event: SubmitEvent) {
    event.preventDefault();
    this.busy = true;
    this.error = "";
    try {
      const pair = await saucerBridge.createIdentity(this.pin);
      this.publicKey = pair.publicKey;
      this.privateKey = pair.privateKey;
      localIdentityAvailable.set(true);
      pushToast("身份已创建，请先备份私钥", "info");
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    } finally {
      this.busy = false;
    }
  }

  private enter() {
    completeLogin({ publicKey: this.publicKey, role: 0 });
  }

  render() {
    return html`
      <form @submit=${this.create}>
        <label>
          设置本地 PIN
          <input
            type="password"
            minlength="4"
            required
            .value=${this.pin}
            @input=${(event: InputEvent) => this.pin = (event.target as HTMLInputElement).value}
          />
        </label>
        <button type="submit" ?disabled=${this.busy}>${this.busy ? "生成中" : "创建新身份"}</button>
      </form>
      ${this.error ? html`<p class="error">${this.error}</p>` : nothing}
      ${this.privateKey
        ? html`
          <section class="backup">
            <p>私钥只显示一次。丢失私钥后，前端无法恢复账号控制权。</p>
            <pre>${this.privateKey}</pre>
            <label class="check">
              <input
                type="checkbox"
                .checked=${this.backedUp}
                @change=${(event: Event) => this.backedUp = (event.target as HTMLInputElement).checked}
              />
              我已完成离线备份
            </label>
            <button type="button" ?disabled=${!this.backedUp} @click=${this.enter}>进入聊天</button>
          </section>
        `
        : nothing}
    `;
  }

  static styles = css`
    form,
    .backup {
      display: grid;
      gap: 12px;
    }

    label {
      display: grid;
      gap: 8px;
      color: #536078;
      font-size: 13px;
      font-weight: 600;
    }

    input {
      box-sizing: border-box;
      width: 100%;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      padding: 12px;
    }

    input[type="checkbox"] {
      width: 16px;
      height: 16px;
      margin: 0;
      flex: 0 0 auto;
      accent-color: #1f8a70;
    }

    button {
      box-sizing: border-box;
      min-height: 42px;
      border: 0;
      border-radius: 6px;
      background: #1f8a70;
      color: #ffffff;
      font-weight: 700;
    }

    button:disabled {
      opacity: 0.55;
      cursor: not-allowed;
    }

    .backup {
      border-top: 1px solid #e4e9f1;
      padding-top: 16px;
    }

    p,
    .error {
      margin: 0;
      font-size: 13px;
      line-height: 1.45;
    }

    p {
      color: #657084;
    }

    .error {
      color: #bf3b3b;
    }

    pre {
      overflow: auto;
      margin: 0;
      border-radius: 6px;
      background: #101824;
      color: #eef5ff;
      padding: 12px;
      white-space: pre-wrap;
      word-break: break-all;
    }

    .check {
      display: flex;
      align-items: center;
      gap: 8px;
      color: #273143;
      line-height: 1.35;
      user-select: none;
    }
  `;
}
