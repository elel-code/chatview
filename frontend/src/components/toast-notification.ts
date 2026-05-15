import { css, html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import { toasts } from "../store";

@customElement("toast-notification")
export class ToastNotification extends SignalWatcher(LitElement) {
  render() {
    return html`
      <div class="stack" aria-live="polite">
        ${toasts.get().map(
          (toast) => html`<div class="toast ${toast.kind}">${toast.text}</div>`,
        )}
      </div>
    `;
  }

  static styles = css`
    .stack {
      position: fixed;
      z-index: 20;
      right: 20px;
      bottom: 20px;
      display: grid;
      gap: 10px;
      width: min(360px, calc(100vw - 32px));
    }

    .toast {
      border: 1px solid #d7dce5;
      border-left: 4px solid #4169e1;
      border-radius: 8px;
      background: #ffffff;
      box-shadow: 0 14px 34px rgba(28, 39, 61, 0.14);
      padding: 12px 14px;
      color: #18202f;
      font-size: 14px;
      line-height: 1.45;
    }

    .toast.success {
      border-left-color: #1f8a70;
    }

    .toast.error {
      border-left-color: #bf3b3b;
    }
  `;
}
