import { css, html, LitElement, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { saucerBridge } from "../../bridge";
import { localIdentityAvailable, pushToast } from "../../store";
import { navigate } from "../../router";

@customElement("auth-import")
export class AuthImport extends LitElement {
  @state()
  private privateKey = "";

  @state()
  private pin = "";

  @state()
  private error = "";

  @state()
  private busy = false;

  private async submit(event: SubmitEvent) {
    event.preventDefault();
    this.busy = true;
    this.error = "";
    try {
      await saucerBridge.importIdentity(this.privateKey, this.pin);
      localIdentityAvailable.set(true);
      pushToast("身份已导入，请用新 PIN 解锁", "success");
      navigate({ name: "auth-unlock" });
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    } finally {
      this.busy = false;
    }
  }

  render() {
    return html`
      <form @submit=${this.submit}>
        <label>
          备份私钥
          <textarea
            required
            rows="4"
            .value=${this.privateKey}
            @input=${(event: InputEvent) => this.privateKey = (event.target as HTMLTextAreaElement).value}
          ></textarea>
        </label>
        <label>
          为本机设置新 PIN
          <input
            type="password"
            minlength="4"
            required
            .value=${this.pin}
            @input=${(event: InputEvent) => this.pin = (event.target as HTMLInputElement).value}
          />
        </label>
        ${this.error ? html`<p class="error">${this.error}</p>` : nothing}
        <button type="submit" ?disabled=${this.busy}>${this.busy ? "导入中" : "导入身份"}</button>
      </form>
    `;
  }

  static styles = css`
    form,
    label {
      display: grid;
      gap: 10px;
    }

    form {
      gap: 12px;
    }

    label {
      color: #536078;
      font-size: 13px;
      font-weight: 600;
    }

    input,
    textarea {
      box-sizing: border-box;
      width: 100%;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      padding: 12px;
      resize: vertical;
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

    .error {
      margin: 0;
      color: #bf3b3b;
      font-size: 13px;
    }
  `;
}
