export interface Identity {
  publicKey: string;
  role: number;
  isLoggedIn: boolean;
}

export interface Friend {
  pubKey: string;
  alias: string;
  isOnline: boolean;
  unread: number;
}

export interface ChatMessage {
  id: string;
  sender: string;
  text: string;
  timestamp: string;
  delivery?: "incoming" | "pending" | "sent" | "failed";
  error?: string;
}

export interface SendMessageResult {
  messageId: string;
  timestamp: string;
  deduplicated?: boolean;
}

export interface MessageHistoryPage {
  messages: ChatMessage[];
  nextCursor?: string;
  hasMore: boolean;
}

export interface MessageHistoryQuery {
  cursor?: string;
  limit?: number;
  direction?: "older" | "newer";
}

export interface AuthLockState {
  lockedUntil?: string;
  remainingAttempts?: number;
}

export interface UserInfo {
  pubKey: string;
  isOnline: boolean;
  isBanned: boolean;
}

export interface AdminStats {
  onlineUsers: number;
  totalUsers: number;
  bannedUsers: number;
}

export interface AdminUpdate {
  users: UserInfo[];
  stats: AdminStats;
}

export interface FriendStatusEvent {
  pubKey: string;
  alias?: string;
  isOnline: boolean;
}

export interface FriendRemovedEvent {
  pubKey: string;
}

export interface OutboxStatus {
  pending: number;
  failed: number;
}

export interface MessageStatusEvent {
  clientMessageId: string;
  delivery: "sent" | "failed";
  serverMessageId?: string;
  timestamp?: string;
  error?: string;
}

export interface CachedMessagesEvent {
  pubKey: string;
  messages: ChatMessage[];
}

export interface Toast {
  id: number;
  kind: "info" | "success" | "error";
  text: string;
}

export interface SaucerExposed {
  hasLocalIdentity(): Promise<boolean>;
  createIdentity(pin: string): Promise<{ publicKey: string; privateKey: string }>;
  importIdentity(privateKey: string, newPin: string): Promise<void>;
  login(pin: string): Promise<{ publicKey: string; role: number }>;
  exportPrivateKey(pin: string): Promise<string>;
  lockSession(): Promise<void>;
  getAuthLockState(): Promise<AuthLockState>;
  listFriends(): Promise<Friend[]>;
  getMessageHistory(pubKey: string, query?: MessageHistoryQuery): Promise<MessageHistoryPage>;
  sendMessage(receiverPubKey: string, text: string, clientMessageId: string): Promise<SendMessageResult | void>;
  markConversationRead(pubKey: string, lastReadServerSeq?: number): Promise<void>;
  addFriend(targetPubKey: string): Promise<void>;
  adminSetUserStatus(targetPubKey: string, status: "banned" | "active"): Promise<void>;
  adminBroadcast(text: string): Promise<void>;
  pollAdminEvents(): Promise<AdminUpdate>;

  // SQLite 发件箱
  getOutboxStatus(): Promise<OutboxStatus>;
  retryOutboxMessage(messageId: string): Promise<void>;
  clearOutbox(): Promise<void>;
}

declare global {
  interface Window {
    saucer?: {
      exposed?: Partial<SaucerExposed>;
    };
  }
}
