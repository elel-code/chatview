import { Router, type RouteDefinition, type RouteGuard } from "@jsr/elelcode__lit-router";
import "@jsr/elelcode__lit-router";
import { identity, isAdmin } from "./store";
import type { Identity } from "./types";

const globalAuthGuard: RouteGuard = ({ leaf }) => {
  if (leaf?.name?.startsWith("auth") || leaf?.name === "root") return true;
  return identity.get().isLoggedIn ? true : { to: "/auth", replace: true };
};

const adminOnlyGuard: RouteGuard = () => {
  return isAdmin.get() ? true : { to: "/chat", replace: true };
};

const baseRoutes: RouteDefinition[] = [
  {
    path: "",
    name: "root",
    component: "root-redirect",
    load: () => import("./pages/root-redirect"),
  },
  {
    path: "auth",
    name: "auth",
    component: "auth-view",
    load: () => import("./pages/auth/auth-view"),
    children: [
      {
        path: "",
        name: "auth-unlock",
        component: "auth-unlock",
        load: () => import("./pages/auth/auth-unlock"),
      },
      {
        path: "create",
        name: "auth-create",
        component: "auth-create",
        load: () => import("./pages/auth/auth-create"),
      },
      {
        path: "import",
        name: "auth-import",
        component: "auth-import",
        load: () => import("./pages/auth/auth-import"),
      },
    ],
  },
];

export const router = new Router({
  basePath: "/",
  routes: baseRoutes,
  beforeRoute: globalAuthGuard,
  autoStart: false,
});

let chatRouteInserted = false;
let adminRouteInserted = false;

export function ensureAppRoutes(role: number) {
  router.batchRouteUpdates(() => {
    if (!chatRouteInserted) {
      router.insertRoutes([
        {
          path: "chat",
          name: "chat",
          component: "main-layout",
          load: () => import("./layouts/main-layout"),
          children: [
            {
              path: "",
              name: "chat-workspace",
              component: "chat-workspace",
              load: () => import("./pages/chat/chat-workspace"),
            },
          ],
        },
      ]);
      chatRouteInserted = true;
    }

    if (role === 1 && !adminRouteInserted) {
      router.insertRoutes(
        [
          {
            path: "admin",
            name: "admin",
            component: "admin-shell",
            guard: adminOnlyGuard,
            load: () => import("./pages/admin/admin-shell"),
            children: [
              {
                path: "",
                name: "admin-dashboard",
                component: "admin-dashboard",
                load: () => import("./pages/admin/admin-dashboard"),
              },
              {
                path: "users",
                name: "admin-users",
                component: "user-manager",
                load: () => import("./pages/admin/user-manager"),
              },
              {
                path: "broadcast",
                name: "admin-broadcast",
                component: "broadcast-panel",
                load: () => import("./pages/admin/broadcast-panel"),
              },
            ],
          },
        ],
        { parentName: "chat" },
      );
      adminRouteInserted = true;
    }
  });
}

export function completeLogin(ident: Pick<Identity, "publicKey" | "role">) {
  identity.set({ ...ident, isLoggedIn: true });
  ensureAppRoutes(ident.role);
  router.push("/chat");
}

export function navigate(path: string) {
  router.push(path);
}
