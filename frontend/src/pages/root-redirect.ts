import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import { replace } from "../router";

@customElement("root-redirect")
export class RootRedirect extends LitElement {
  connectedCallback() {
    super.connectedCallback();
    queueMicrotask(() => replace({ name: "auth-unlock" }));
  }

  render() {
    return html``;
  }
}
