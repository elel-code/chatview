import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import { router } from "../router";

@customElement("root-redirect")
export class RootRedirect extends LitElement {
  connectedCallback() {
    super.connectedCallback();
    queueMicrotask(() => router.replace("/auth"));
  }

  render() {
    return html``;
  }
}
