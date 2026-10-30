import { expect, type Browser, type BrowserContext, type Page } from "@playwright/test";
import { ADMIN_EMAIL, ADMIN_PASSWORD } from "./env";

export async function uiLogin(page: Page, email = ADMIN_EMAIL, password = ADMIN_PASSWORD) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/$/);
}

export async function uiLogout(page: Page) {
  await page.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login/);
}

const sessions = new Map<string, Awaited<ReturnType<BrowserContext["storageState"]>>>();

export async function userContext(browser: Browser, email: string, password: string): Promise<BrowserContext> {
  const cached = sessions.get(email);
  if (cached) return browser.newContext({ storageState: cached });
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await uiLogin(page, email, password);
  sessions.set(email, await ctx.storageState());
  await page.close();
  return ctx;
}
