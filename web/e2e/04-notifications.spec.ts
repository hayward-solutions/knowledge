import { expect, test } from "@playwright/test";
import { ADMIN_STATE } from "./support/env";
import { adminApi, createPublishedPage, ensureSpace, unreadCount, type ApiPage } from "./support/api";
import { editorSurface, publishFromEditor, waitForSaved } from "./support/editor";
import { run, spaceKey, uniq } from "./support/run";
import { uiLogin } from "./support/ui";

const second = () => ({
  name: `Grace Hopper ${run().runId}`,
  email: `grace.${run().runId}@example.com`,
  password: "Grace-Hopper-Pass-1!",
});
const title = () => uniq("Release Plan");

let watched: ApiPage;

test.describe.serial("watching and notifications", () => {
  test.use({ storageState: ADMIN_STATE });

  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    watched = await createPublishedPage(api, spaceKey(), title(), "<p>Initial release plan.</p>");
    await api.dispose();
  });

  const pageUrl = () => `/spaces/${spaceKey()}/pages/${watched.id}/${watched.slug}`;

  test("admin creates a second user from the admin console", async ({ page }) => {
    const u = second();
    await page.goto("/admin/users");
    await page.getByRole("button", { name: "New user" }).click();
    const dialog = page.getByRole("dialog", { name: "Create user" });
    await dialog.getByLabel("Display name").fill(u.name);
    await dialog.getByLabel("Email").fill(u.email);
    await dialog.getByLabel("Password").fill(u.password);
    await dialog.getByRole("button", { name: "Create user" }).click();
    await expect(page.getByText(u.email)).toBeVisible();
  });

  test("admin grants the second user editor access to the space", async ({ page }) => {
    const u = second();
    await page.goto(`/spaces/${spaceKey()}/settings/members`);
    await page.getByPlaceholder("Search by name or email").fill(u.name);
    await page.getByRole("option", { name: new RegExp(u.name) }).click();
    await page.locator("form").filter({ hasText: "Add a member" }).locator("select").selectOption("editor");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    await expect(page.getByRole("region", { name: "Members" }).getByText(u.name)).toBeVisible();
  });

  test("admin can toggle watching a page", async ({ page }) => {
    await page.goto(pageUrl());
    const stop = page.getByRole("button", { name: "Stop watching" });
    const watch = page.getByRole("button", { name: "Watch", exact: true });
    await expect(stop.or(watch)).toBeVisible();
    if (await stop.isVisible()) {
      await stop.click();
      await expect(watch).toBeVisible();
    }
    await watch.click();
    await expect(stop).toBeVisible();
    await page.reload();
    await expect(page.getByRole("button", { name: "Stop watching" })).toBeVisible();
  });

  test("a change published by the second user notifies the watcher", async ({ browser, page }) => {
    const u = second();
    const api = await adminApi();
    const before = await unreadCount(api);

    await page.goto(pageUrl());
    await expect(page.getByRole("button", { name: /^Notifications/ })).toBeVisible();

    const userContext = await browser.newContext();
    const userPage = await userContext.newPage();
    await uiLogin(userPage, u.email, u.password);
    await userPage.goto(`/spaces/${spaceKey()}/pages/${watched.id}/edit`);
    const surface = editorSurface(userPage);
    await expect(surface).toContainText("Initial release plan.");
    await surface.locator("p").first().click();
    await userPage.keyboard.press("End");
    await userPage.keyboard.type(" Updated by the second user.");
    await waitForSaved(userPage);
    await publishFromEditor(userPage, "Second user update");
    await expect(userPage.locator(".kn-content").first()).toContainText("Updated by the second user.");
    await userContext.close();

    const bell = page.getByRole("button", { name: new RegExp(`^Notifications, ${before + 1} unread`) });
    await expect(bell).toBeVisible({ timeout: 20_000 });
    expect(await unreadCount(api)).toBe(before + 1);

    await bell.click();
    await expect(page.getByText(`${u.name} published`).first()).toBeVisible();
    await expect(page.getByText(title()).first()).toBeVisible();
    await api.dispose();
  });

  test("notifications page lists it and mark all read clears the badge", async ({ page }) => {
    const u = second();
    await page.goto("/notifications");
    const item = page.getByRole("link").filter({ hasText: new RegExp(`${u.name}.*published.*${title()}`) });
    await expect(item.first()).toBeVisible();
    await page.getByRole("button", { name: "Mark all read" }).first().click();
    await expect(page.getByRole("button", { name: "Notifications", exact: true })).toBeVisible();
    const api = await adminApi();
    expect(await unreadCount(api)).toBe(0);
    await page.getByRole("tab", { name: "Unread" }).click();
    await expect(item).toHaveCount(0);
    await api.dispose();
  });

  test("the second user sees the published change and the history entry", async ({ browser }) => {
    const u = second();
    const ctx = await browser.newContext();
    const p = await ctx.newPage();
    await uiLogin(p, u.email, u.password);
    await p.goto(pageUrl());
    await expect(p.locator(".kn-content").first()).toContainText("Updated by the second user.");
    await expect(p.getByRole("link", { name: "Version 2" })).toBeVisible();
    await ctx.close();
  });

  test("mentioning a user in a published change notifies them", async ({ browser, page }) => {
    const u = second();
    const ctx = await browser.newContext();
    const p = await ctx.newPage();
    await uiLogin(p, u.email, u.password);
    await p.goto(`/spaces/${spaceKey()}/pages/${watched.id}/edit`);
    const surface = editorSurface(p);
    await expect(surface).toContainText("Updated by the second user.");
    await surface.locator("p").first().click();
    await p.keyboard.press("End");
    await p.keyboard.type(" Thanks @Adm");
    await expect(p.getByRole("option", { name: /Admin/ })).toBeVisible();
    await p.keyboard.press("Enter");
    await waitForSaved(p);
    await publishFromEditor(p, "Mention the admin");
    await ctx.close();

    await page.goto("/notifications");
    const item = page.getByRole("link").filter({ hasText: new RegExp(`${u.name}.*mentioned you in.*${title()}`) });
    await expect(item.first()).toBeVisible({ timeout: 15_000 });
  });
});
