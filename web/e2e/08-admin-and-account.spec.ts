import { expect, test } from "@playwright/test";
import { ADMIN_STATE } from "./support/env";
import { adminApi, createPublishedPage, ensureSpace, setLabels } from "./support/api";
import { run, spaceKey } from "./support/run";
import { uiLogin } from "./support/ui";

test.use({ storageState: ADMIN_STATE });

const user = () => ({
  name: `Linus Torvalds ${run().runId}`,
  email: `linus.${run().runId}@example.com`,
  password: "Linus-Torvalds-Pass-1!",
  resetPassword: "Linus-Reset-Pass-2!",
  accountPassword: "Linus-Account-Pass-3!",
});

test.describe.serial("admin console and account settings", () => {
  test("admin creates a user and resets the password", async ({ page, browser }) => {
    const u = user();
    await page.goto("/admin/users");
    await page.getByRole("button", { name: "New user" }).click();
    const create = page.getByRole("dialog", { name: "Create user" });
    await create.getByLabel("Display name").fill(u.name);
    await create.getByLabel("Email").fill(u.email);
    await create.getByLabel("Password").fill(u.password);
    await create.getByRole("button", { name: "Create user" }).click();
    await expect(page.getByText(u.email)).toBeVisible();

    const row = page.getByRole("row").filter({ hasText: u.email });
    await row.getByRole("button", { name: "Reset password" }).click();
    const reset = page.getByRole("dialog", { name: new RegExp(`Reset password for ${u.name}`) });
    await reset.getByLabel("New password").fill(u.resetPassword);
    await reset.getByRole("button", { name: "Set password" }).click();
    await expect(page.getByText("Password updated")).toBeVisible();

    const ctx = await browser.newContext();
    const other = await ctx.newPage();
    await other.goto("/login");
    await other.getByLabel("Email").fill(u.email);
    await other.getByLabel("Password").fill(u.password);
    await other.getByRole("button", { name: "Sign in" }).click();
    await expect(other.getByRole("alert")).toBeVisible();
    await other.getByLabel("Password").fill(u.resetPassword);
    await other.getByRole("button", { name: "Sign in" }).click();
    await expect(other).toHaveURL(/\/$/);
    await ctx.close();
  });

  test("admin can grant and revoke system admin and disable a user", async ({ page, browser }) => {
    const u = user();
    await page.goto("/admin/users");
    const row = page.getByRole("row").filter({ hasText: u.email });
    const box = row.getByRole("checkbox", { name: `System administrator: ${u.name}` });
    await box.click();
    await expect(box).toBeChecked();
    await page.reload();
    await expect(page.getByRole("row").filter({ hasText: u.email }).getByRole("checkbox")).toBeChecked();
    await page.getByRole("row").filter({ hasText: u.email }).getByRole("checkbox").click();
    await expect(page.getByRole("row").filter({ hasText: u.email }).getByRole("checkbox")).not.toBeChecked();

    await page.getByRole("row").filter({ hasText: u.email }).getByRole("button", { name: "Disable" }).click();
    await page.getByRole("dialog", { name: "Disable user" }).getByRole("button", { name: "Disable" }).click();
    await expect(page.getByRole("row").filter({ hasText: u.email }).getByText("Disabled")).toBeVisible();

    const ctx = await browser.newContext();
    const other = await ctx.newPage();
    await other.goto("/login");
    await other.getByLabel("Email").fill(u.email);
    await other.getByLabel("Password").fill(u.resetPassword);
    await other.getByRole("button", { name: "Sign in" }).click();
    await expect(other.getByRole("alert")).toBeVisible();
    await ctx.close();

    await page.getByRole("row").filter({ hasText: u.email }).getByRole("button", { name: "Enable" }).click();
    await expect(page.getByRole("row").filter({ hasText: u.email }).getByText("Active")).toBeVisible();
  });

  test("a user can change their display name and password from the account page", async ({ browser }) => {
    const u = user();
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await uiLogin(page, u.email, u.resetPassword);
    await page.goto("/account");
    await expect(page.getByRole("heading", { name: "Account" })).toBeVisible();
    const renamed = `${u.name} (renamed)`;
    await page.getByLabel("Display name").fill(renamed);
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByText(renamed).first()).toBeVisible();

    await page.getByLabel("Current password").fill(u.resetPassword);
    await page.getByLabel("New password", { exact: true }).fill(u.accountPassword);
    await page.getByLabel("Confirm new password").fill(u.accountPassword);
    await page.getByRole("button", { name: "Change password" }).click();
    await expect(page.getByText(/password (changed|updated)/i).first()).toBeVisible();
    await ctx.close();

    const api = await adminApi();
    const res = await api.get("admin/users");
    const users = (await res.json()) as { email: string; display_name: string }[];
    expect(users.find((x) => x.email === u.email)?.display_name).toBe(renamed);
    await api.dispose();
  });

  test("admin spaces, templates and labels pages render live data", async ({ page }) => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    const labelled = await createPublishedPage(api, spaceKey(), "Admin label page", "<p>labels</p>");
    await setLabels(api, labelled.id, [`admin-${run().runId}`]);
    await api.dispose();

    await page.goto("/admin/spaces");
    await expect(page.getByRole("heading", { name: "Spaces" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: spaceKey() })).toBeVisible();

    await page.goto("/admin/templates");
    await expect(page.getByRole("heading", { name: "Global templates" })).toBeVisible();
    await expect(page.getByText("Meeting notes")).toBeVisible();

    await page.goto("/admin/labels");
    await expect(page.getByRole("heading", { name: "Labels" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: `admin-${run().runId}` })).toBeVisible();
  });

  test("non-admin users cannot open the admin console", async ({ browser }) => {
    const u = user();
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await uiLogin(page, u.email, u.accountPassword);
    await page.goto("/admin/users");
    await expect(page.getByRole("button", { name: "New user" })).toHaveCount(0);
    await ctx.close();
  });
});
