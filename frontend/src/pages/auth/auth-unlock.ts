import { css, html, LitElement, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import { saucerBridge } from "../../bridge";
import { completeLogin, navigate } from "../../router";
import { authLockState, localIdentityAvailable, pushToast } from "../../store";

@customElement("auth-unlock")
export class AuthUnlock extends SignalWatcher(LitElement) {
  @state()
  private pin = "";

  @state()
  private error = "";

  @state()
  private busy = false;

  connectedCallback() {
    super.connectedCallback();
    void this.refreshLockState();
  }

  private async refreshLockState() {
    try {
      authLockState.set(await saucerBridge.getAuthLockState());
    } catch {
      authLockState.set({});
    }
  }

  private async submit(event: SubmitEvent) {
    event.preventDefault();
    this.busy = true;
    this.error = "";
    try {
      const ident = await saucerBridge.login(this.pin);
      authLockState.set({});
      completeLogin(ident);
      pushToast("身份已解锁", "success");
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
      void this.refreshLockState();
    } finally {
      this.busy = false;
    }
  }

  render() {
    const hasLocalIdentity = localIdentityAvailable.get();
    const lockState = authLockState.get();
    const locked = lockState.lockedUntil && new Date(lockState.lockedUntil).getTime() > Date.now();
    return html`
      <form @submit=${this.submit}>
        ${hasLocalIdentity === false
          ? html`<p class="warning">未检测到本机加密身份。请创建新身份或导入已有私钥。</p>`
          : nothing}
        ${locked
          ? html`<p class="warning">PIN 已临时锁定，请在 ${new Date(lockState.lockedUntil!).toLocaleTimeString("zh-CN")} 后重试。</p>`
          : lockState.remainingAttempts !== undefined
            ? html`<p class="hint">剩余尝试次数：${lockState.remainingAttempts}</p>`
            : nothing}
        <label>
          本地 PIN
          <input
            type="password"
            autocomplete="current-password"
            minlength="4"
            required
            .value=${this.pin}
            @input=${(event: InputEvent) => this.pin = (event.target as HTMLInputElement).value}
          />
        </label>
        ${this.error ? html`<p class="error">${this.error}</p>` : nothing}
        <button type="submit" ?disabled=${this.busy || Boolean(locked)}>${this.busy ? "解锁中" : "解锁进入"}</button>
        <p class="hint">PIN 只用于本机解密，登录签名由原生层完成。</p>
        <button type="button" class="link" @click=${() => navigate({ name: "auth-create" })}>还没有身份，创建一个</button>
      </form>
    `;
  }

  static styles = css`
    form,
    label {
      display: grid;
      gap: 10px;
    }

    label {
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
      color: #18202f;
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
      opacity: 0.65;
    }

    .link {
      background: transparent;
      color: #4169e1;
      font-weight: 600;
    }

    .hint,
    .error,
    .warning {
      margin: 0;
      font-size: 13px;
      line-height: 1.45;
    }

    .hint {
      color: #657084;
    }

    .error {
      color: #bf3b3b;
    }

    .warning {
      border: 1px solid #ead6a8;
      border-radius: 6px;
      background: #fff8e8;
      color: #7b5b13;
      padding: 10px 12px;
    }
  `;
}
