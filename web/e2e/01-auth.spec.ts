import { expect, test } from "@playwright/test";
import { API_URL, ADMIN_EMAIL, ADMIN_PASSWORD } from "./support/env";
import { uiLogin, uiLogout } from "./support/ui";

test.describe("authentication", () => {
  test("unauthenticated visitors are sent to the login page", async ({ page }) => {
    await page.goto("/spaces");
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole("heading", { name: "Sign in to continue" })).toBeVisible();
  });

  test("wrong password shows an error", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Email").fill(ADMIN_EMAIL);
    await page.getByLabel("Password").fill("Definitely-Wrong-1!");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });

  test("admin signs in and out", async ({ page }) => {
    await uiLogin(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
    await uiLogout(page);
    await page.goto("/spaces");
    await expect(page).toHaveURL(/\/login/);
  });

  test("OIDC config endpoint reports disabled and no SSO button is rendered", async ({
    page,
    request,
  }) => {
    const res = await request.get(`${API_URL}/api/v1/auth/oidc/config`);
    expect(res.ok()).toBeTruthy();
    expect(await res.json()).toEqual({ enabled: false });
    await page.goto("/login");
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
    await expect(page.getByText(/Sign in with/)).toHaveCount(0);
  });

  test("health endpoint answers", async ({ request }) => {
    const res = await request.get(`${API_URL}/health`);
    expect(res.ok()).toBeTruthy();
    expect((await res.json()).ok).toBe(true);
  });
});
