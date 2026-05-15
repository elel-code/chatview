import { Signal } from "@lit-labs/signals";
import type {
  AdminStats,
  AuthLockState,
  ChatMessage,
  Friend,
  Identity,
  OutboxStatus,
  Toast,
  UserInfo,
} from "./types";

export const identity = new Signal.State<Identity>({
  publicKey: "",
  role: 0,
  isLoggedIn: false,
});

export const isAdmin = new Signal.Computed(() => identity.get().role === 1);
const activeChatStorageKey = "chatview.activeChatPubKey";

export const activeChatPubKey = new Signal.State(
  localStorage.getItem(activeChatStorageKey) ?? "",
);

export const friends = new Signal.State<Friend[]>([]);
export const friendListStatus = new Signal.State<{
  loading: boolean;
  error: string;
}>({ loading: false, error: "" });

export const chatData = new Signal.State<Record<string, ChatMessage[]>>({});
export const chatHistoryState = new Signal.State<
  Record<string, { loaded: boolean; loading: boolean; nextCursor?: string; hasMore: boolean }>
>({});

export const adminUsers = new Signal.State<UserInfo[]>([]);
export const adminStatus = new Signal.State<{
  dashboardLoading: boolean;
  usersLoading: boolean;
  error: string;
}>({ dashboardLoading: false, usersLoading: false, error: "" });
export const adminStats = new Signal.State<AdminStats>({
  onlineUsers: 0,
  totalUsers: 0,
  bannedUsers: 0,
});

export const localIdentityAvailable = new Signal.State<boolean | null>(null);
export const authLockState = new Signal.State<AuthLockState>({});
export const loadingRoute = new Signal.State(false);
export const outboxStatus = new Signal.State<OutboxStatus>({ pending: 0, failed: 0 });
export const toasts = new Signal.State<Toast[]>([]);

let toastId = 0;

export function pushToast(text: string, kind: Toast["kind"] = "info") {
  const toast: Toast = { id: ++toastId, kind, text };
  toasts.set([...toasts.get(), toast]);
  window.setTimeout(() => {
    toasts.set(toasts.get().filter((item) => item.id !== toast.id));
  }, 4200);
}

export function resetSession() {
  identity.set({ publicKey: "", role: 0, isLoggedIn: false });
  setActiveChat("");
}

export function setActiveChat(pubKey: string) {
  activeChatPubKey.set(pubKey);
  if (pubKey) {
    localStorage.setItem(activeChatStorageKey, pubKey);
  } else {
    localStorage.removeItem(activeChatStorageKey);
  }
}

export function setFriends(nextFriends: Friend[]) {
  const unique = new Map<string, Friend>();
  for (const friend of nextFriends) {
    unique.set(friend.pubKey, friend);
  }
  const normalized = [...unique.values()];
  friends.set(normalized);
  const active = activeChatPubKey.get();
  if ((!active || !unique.has(active)) && normalized[0]) {
    setActiveChat(normalized[0].pubKey);
  }
}

export function upsertFriend(friend: Friend) {
  const next = [...friends.get()];
  const index = next.findIndex((item) => item.pubKey === friend.pubKey);
  if (index >= 0) {
    next[index] = { ...next[index], ...friend };
  } else {
    next.push(friend);
  }
  friends.set(next);
}

export function removeFriend(pubKey: string) {
  const next = friends.get().filter((friend) => friend.pubKey !== pubKey);
  friends.set(next);
  if (activeChatPubKey.get() === pubKey) {
    setActiveChat(next[0]?.pubKey ?? "");
  }
}

export function setFriendUnread(pubKey: string, unread: number) {
  friends.set(
    friends.get().map((friend) =>
      friend.pubKey === pubKey ? { ...friend, unread } : friend,
    ),
  );
}

export function appendMessages(pubKey: string, messages: ChatMessage[]) {
  const current = chatData.get()[pubKey] ?? [];
  chatData.set({ ...chatData.get(), [pubKey]: mergeMessages(current, messages) });
}

export function prependMessages(pubKey: string, messages: ChatMessage[]) {
  const current = chatData.get()[pubKey] ?? [];
  chatData.set({ ...chatData.get(), [pubKey]: mergeMessages(messages, current) });
}

export function updateMessage(pubKey: string, messageId: string, patch: Partial<ChatMessage>) {
  const current = chatData.get()[pubKey] ?? [];
  chatData.set({
    ...chatData.get(),
    [pubKey]: current.map((message) =>
      message.id === messageId ? { ...message, ...patch } : message,
    ),
  });
}

export function updateMessageById(messageId: string, patch: Partial<ChatMessage>) {
  const next = { ...chatData.get() };
  for (const [pubKey, messages] of Object.entries(next)) {
    const index = messages.findIndex((message) => message.id === messageId);
    if (index === -1) continue;
    const updated = [...messages];
    updated[index] = { ...updated[index], ...patch };
    next[pubKey] = updated;
  }
  chatData.set(next);
}

export function setChatHistoryState(
  pubKey: string,
  patch: Partial<{ loaded: boolean; loading: boolean; nextCursor?: string; hasMore: boolean }>,
) {
  const current = chatHistoryState.get()[pubKey] ?? { loaded: false, loading: false, hasMore: true };
  chatHistoryState.set({
    ...chatHistoryState.get(),
    [pubKey]: { ...current, ...patch },
  });
}

function mergeMessages(first: ChatMessage[], second: ChatMessage[]) {
  const byId = new Map<string, ChatMessage>();
  for (const message of [...first, ...second]) {
    byId.set(message.id, message);
  }
  return [...byId.values()];
}
