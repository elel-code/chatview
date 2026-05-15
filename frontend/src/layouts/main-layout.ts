import { css, html, LitElement } from "lit";
import { customElement, property } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import type { RouteContext } from "@jsr/elelcode__lit-router";
import "../components/identity-card";
import "./sidebar-menu";

@customElement("main-layout")
export class MainLayout extends SignalWatcher(LitElement) {
  @property({ attribute: false })
  accessor routeContext: RouteContext | undefined;

  render() {
    return html`
      <div class="layout">
        <sidebar-menu></sidebar-menu>
        <main>
          <slot name="route-child"></slot>
        </main>
      </div>
    `;
  }

  static styles = css`
    .layout {
      display: grid;
      grid-template-columns: 300px minmax(0, 1fr);
      min-height: 100svh;
      background: #f7f8fb;
    }

    main {
      min-width: 0;
      min-height: 100svh;
    }

    @media (max-width: 820px) {
      .layout {
        grid-template-columns: 1fr;
      }

      main {
        min-height: auto;
      }
    }
  `;
}
