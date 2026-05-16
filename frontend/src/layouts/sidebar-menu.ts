import { css, html, LitElement, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { SignalWatcher } from "@lit-labs/signals";
import { saucerBridge } from "../bridge";
import { navigate } from "../router";
import { friends, isAdmin, pushToast, resetSession } from "../store";

@customElement("sidebar-menu")
export class SidebarMenu extends SignalWatcher(LitElement) {
  @state()
  private friendKey = "";

  private async addFriend(event: SubmitEvent) {
    event.preventDefault();
    try {
      await saucerBridge.addFriend(this.friendKey);
      pushToast("好友已提交", "success");
      this.friendKey = "";
    } catch (error) {
      pushToast(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private async logout() {
    try {
      await saucerBridge.lockSession();
    } catch (error) {
      pushToast(error instanceof Error ? error.message : String(error), "error");
    } finally {
      resetSession();
      navigate({ name: "auth-unlock" });
    }
  }

  render() {
    const onlineCount = friends.get().filter((friend) => friend.isOnline).length;
    return html`
      <aside>
        <header>
          <strong>ChatView</strong>
          <span>${onlineCount} 位好友在线</span>
        </header>
        <identity-card></identity-card>
        <nav>
          <button type="button" @click=${() => navigate({ name: "chat" })}>聊天</button>
          ${isAdmin.get()
            ? html`<button type="button" @click=${() => navigate({ name: "admin-dashboard" })}>管理员</button>`
            : nothing}
        </nav>
        <form @submit=${this.addFriend}>
          <label>
            添加好友
            <input
              placeholder="粘贴目标公钥"
              .value=${this.friendKey}
              @input=${(event: InputEvent) => this.friendKey = (event.target as HTMLInputElement).value}
            />
          </label>
          <button type="submit">添加</button>
        </form>
        <button class="logout" type="button" @click=${this.logout}>锁定</button>
      </aside>
    `;
  }

  static styles = css`
    aside {
      position: sticky;
      top: 0;
      display: grid;
      grid-template-rows: auto auto auto auto 1fr;
      gap: 16px;
      height: 100svh;
      border-right: 1px solid #dce2ec;
      background: #eef2f7;
      padding: 18px;
    }

    header {
      display: grid;
      gap: 4px;
    }

    header strong {
      color: #18202f;
      font-size: 20px;
    }

    header span {
      color: #657084;
      font-size: 13px;
    }

    nav {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 8px;
    }

    button {
      min-height: 38px;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      background: #ffffff;
      color: #273143;
      font-weight: 650;
    }

    form,
    label {
      display: grid;
      gap: 8px;
    }

    label {
      color: #536078;
      font-size: 13px;
      font-weight: 650;
    }

    input {
      box-sizing: border-box;
      min-width: 0;
      border: 1px solid #cfd6e2;
      border-radius: 6px;
      padding: 10px;
    }

    .logout {
      align-self: end;
      background: #273143;
      color: #ffffff;
    }

    @media (max-width: 820px) {
      aside {
        position: static;
        height: auto;
        border-right: 0;
        border-bottom: 1px solid #dce2ec;
      }
    }
  `;
}
