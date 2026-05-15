import type {
  AdminUpdate,
  AuthLockState,
  ChatMessage,
  Friend,
  MessageHistoryQuery,
  OutboxStatus,
  SaucerExposed,
  UserInfo,
} from "./types";

const wait = (ms = 220) => new Promise((resolve) => window.setTimeout(resolve, ms));

const mockUsers: UserInfo[] = [
  { pubKey: "pk_admin_DEMO", isOnline: true, isBanned: false },
  { pubKey: "pk_alice_7F4A", isOnline: true, isBanned: false },
  { pubKey: "pk_ops_19BC", isOnline: false, isBanned: false },
  { pubKey: "pk_spam_9821", isOnline: false, isBanned: true },
];

const pendingMessages = new Map<string, ChatMessage[]>();
const mockOutbox = new Map<string, { receiverPubKey: string; text: string; failed: boolean }>();
let mockLockState: AuthLockState = { remainingAttempts: 5 };
const defaultFriends: Friend[] = [
  { pubKey: "pk_alice_7F4A", alias: "Alice", isOnline: true, unread: 0 },
  { pubKey: "pk_ops_19BC", alias: "Ops Room", isOnline: false, unread: 0 },
];

function loadMockFriends() {
  const raw = localStorage.getItem("chatview.mockFriends");
  return raw ? (JSON.parse(raw) as Friend[]) : defaultFriends;
}

function saveMockFriends(friends: Friend[]) {
  localStorage.setItem("chatview.mockFriends", JSON.stringify(friends));
}

function randomKey(prefix: string) {
  return `${prefix}_${crypto.getRandomValues(new Uint32Array(1))[0].toString(16).toUpperCase()}`;
}

function getAdminUpdate(): AdminUpdate {
  return {
    users: mockUsers,
    stats: {
      onlineUsers: mockUsers.filter((user) => user.isOnline).length,
      totalUsers: mockUsers.length,
      bannedUsers: mockUsers.filter((user) => user.isBanned).length,
    },
  };
}

const mockExposed: SaucerExposed = {
  async hasLocalIdentity() {
    await wait(80);
    return localStorage.getItem("chatview.identity") !== null;
  },

  async createIdentity(pin: string) {
    await wait(300);
    if (pin.length < 4) throw new Error("PIN 至少需要 4 位");
    const publicKey = randomKey("pk");
    const privateKey = randomKey("sk");
    localStorage.setItem("chatview.identity", JSON.stringify({ publicKey, privateKey, pin, role: 0 }));
    return { publicKey, privateKey };
  },

  async importIdentity(privateKey: string, newPin: string) {
    await wait(240);
    if (!privateKey.trim().startsWith("sk_")) throw new Error("invalid key format");
    if (newPin.length < 4) throw new Error("PIN 至少需要 4 位");
    localStorage.setItem(
      "chatview.identity",
      JSON.stringify({ publicKey: randomKey("pk_imported"), privateKey, pin: newPin, role: 0 }),
    );
  },

  async login(pin: string) {
    await wait(260);
    if (mockLockState.lockedUntil && new Date(mockLockState.lockedUntil).getTime() > Date.now()) {
      throw new Error("too many attempts");
    }
    const stored = localStorage.getItem("chatview.identity");
    const record = stored
      ? (JSON.parse(stored) as { publicKey: string; pin: string; role: number })
      : { publicKey: "pk_admin_DEMO", pin: "0000", role: 1 };
    if (pin !== record.pin) {
      const remaining = Math.max((mockLockState.remainingAttempts ?? 5) - 1, 0);
      mockLockState = remaining === 0
        ? { remainingAttempts: 0, lockedUntil: new Date(Date.now() + 30_000).toISOString() }
        : { remainingAttempts: remaining };
      throw new Error("wrong pin");
    }
    mockLockState = { remainingAttempts: 5 };
    return { publicKey: record.publicKey, role: record.role };
  },

  async exportPrivateKey(pin: string) {
    await wait(180);
    const stored = localStorage.getItem("chatview.identity");
    const record = stored
      ? (JSON.parse(stored) as { privateKey: string; pin: string })
      : { privateKey: "sk_demo_private_key_only_for_local_preview", pin: "0000" };
    if (pin !== record.pin) throw new Error("wrong pin");
    return record.privateKey;
  },

  async lockSession() {
    await wait(80);
  },

  async getAuthLockState() {
    await wait(60);
    if (mockLockState.lockedUntil && new Date(mockLockState.lockedUntil).getTime() <= Date.now()) {
      mockLockState = { remainingAttempts: 5 };
    }
    return mockLockState;
  },

  async listFriends() {
    await wait(120);
    return loadMockFriends();
  },

  async getMessageHistory(pubKey: string, query: MessageHistoryQuery = {}) {
    const { cursor, direction = "older", limit = 30 } = query;
    await wait(80); // 缓存命中 — 快速返回

    if (direction === "newer") {
      const messages = pendingMessages.get(pubKey) ?? [];
      pendingMessages.delete(pubKey);
      return {
        messages,
        hasMore: false,
      };
    }

    if (!cursor) {
      // 模拟 SQLite 缓存: 返回最近 5 条消息
      const cached = Array.from({ length: 5 }, (_, index) => {
        const serial = index + 1;
        return {
          id: `cached-${pubKey}-${serial}`,
          sender: index % 2 === 0 ? pubKey : "me",
          text: `[缓存] 消息 ${serial}`,
          timestamp: new Date(Date.now() - 1000 * 60 * (serial + 5)).toISOString(),
          delivery: index % 2 === 0 ? "incoming" : "sent",
        } satisfies ChatMessage;
      });

      // 模拟后台 gRPC 刷新: 延迟后 dispatch 新消息
      window.setTimeout(() => {
        const fresh = Array.from({ length: 2 }, (_, index) => {
          const serial = index + 6;
          return {
            id: `fresh-${pubKey}-${serial}`,
            sender: index % 2 === 0 ? pubKey : "me",
            text: `[服务端] 新消息 ${serial}`,
            timestamp: new Date(Date.now() - 1000 * 60 * index).toISOString(),
            delivery: index % 2 === 0 ? "incoming" : "sent",
          } satisfies ChatMessage;
        });
        window.dispatchEvent(new CustomEvent("saucer:cached-messages", {
          detail: { pubKey, messages: fresh },
        }));
      }, 600);

      return {
        messages: cached,
        nextCursor: "1",
        hasMore: true,
      };
    }

    // cursor 非空: 模拟 gRPC 翻页 (旧数据不在缓存中)
    await wait(180);
    const page = Number(cursor);
    const messages = Array.from({ length: Math.min(limit, 5) }, (_, index) => {
      const serial = page * limit + index + 1;
      return {
        id: `history-${pubKey}-${serial}`,
        sender: index % 2 === 0 ? pubKey : "me",
        text: `历史消息 ${serial}`,
        timestamp: new Date(Date.now() - 1000 * 60 * (serial + 120)).toISOString(),
        delivery: index % 2 === 0 ? "incoming" : "sent",
      } satisfies ChatMessage;
    });
    return {
      messages,
      nextCursor: page < 1 ? String(page + 1) : undefined,
      hasMore: page < 1,
    };
  },

  async sendMessage(receiverPubKey: string, text: string, clientMessageId: string) {
    await wait(140);
    if (text.includes("/fail")) {
      mockOutbox.set(clientMessageId, { receiverPubKey, text, failed: true });
      throw new Error("mock send failed");
    }
    const timestamp = new Date().toISOString();
    const messageId = crypto.randomUUID();
    const reply: ChatMessage = {
      id: crypto.randomUUID(),
      sender: receiverPubKey,
      text: `已收到：${text}`,
      timestamp: new Date(Date.now() + 400).toISOString(),
      delivery: "incoming",
    };
    pendingMessages.set(receiverPubKey, [...(pendingMessages.get(receiverPubKey) ?? []), reply]);
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent("saucer:messages-pending", { detail: receiverPubKey }));
    }, 500);
    return { messageId, timestamp };
  },

  async markConversationRead() {
    await wait(40);
  },

  async addFriend(targetPubKey: string) {
    await wait(180);
    const pubKey = targetPubKey.trim();
    if (!pubKey) throw new Error("public key required");
    const nextFriend: Friend = {
      pubKey,
      alias: pubKey.slice(0, 12),
      isOnline: false,
      unread: 0,
    };
    const next = [...loadMockFriends().filter((friend) => friend.pubKey !== pubKey), nextFriend];
    saveMockFriends(next);
    window.dispatchEvent(
      new CustomEvent("saucer:friend-status", {
        detail: nextFriend,
      }),
    );
  },

  async adminSetUserStatus(targetPubKey: string, status: "banned" | "active") {
    await wait(180);
    const user = mockUsers.find((item) => item.pubKey === targetPubKey);
    if (!user) throw new Error("user not found");
    user.isBanned = status === "banned";
  },

  async adminBroadcast(text: string) {
    await wait(160);
    window.dispatchEvent(new CustomEvent("saucer:system-broadcast", { detail: text }));
  },

  async pollAdminEvents() {
    await wait(180);
    return getAdminUpdate();
  },

  // ── SQLite 发件箱 (mock) ────────────────────

  async getOutboxStatus() {
    await wait(60);
    const status: OutboxStatus = { pending: 0, failed: 0 };
    for (const item of mockOutbox.values()) {
      if (item.failed) status.failed += 1;
      else status.pending += 1;
    }
    return status;
  },

  async retryOutboxMessage(messageId: string) {
    await wait(100);
    const item = mockOutbox.get(messageId);
    if (!item) return;
    if (item.text.includes("/fail")) {
      window.dispatchEvent(new CustomEvent("saucer:message-status", {
        detail: { clientMessageId: messageId, delivery: "failed", error: "mock send failed" },
      }));
      throw new Error("mock send failed");
    }
    mockOutbox.delete(messageId);
    window.dispatchEvent(new CustomEvent("saucer:message-status", {
      detail: {
        clientMessageId: messageId,
        delivery: "sent",
        serverMessageId: messageId,
        timestamp: new Date().toISOString(),
      },
    }));
  },

  async clearOutbox() {
    await wait(60);
    mockOutbox.clear();
  },
};

export function isMockBridgeActive() {
  return !window.saucer?.exposed;
}

export const saucerBridge: SaucerExposed = new Proxy(mockExposed, {
  get(target, property: keyof SaucerExposed) {
    const exposed = window.saucer?.exposed;
    const nativeFn = exposed?.[property];
    return typeof nativeFn === "function" ? nativeFn.bind(exposed) : target[property];
  },
});
