import path from "node:path";
import { expect, test } from "@playwright/test";
import { ADMIN_STATE, FIXTURE_DIR } from "./support/env";
import { adminApi, createPublishedPage, ensureSpace } from "./support/api";
import { editorSurface } from "./support/editor";
import { spaceKey, uniq } from "./support/run";

test.use({ storageState: ADMIN_STATE });

test.describe.serial("templates, activity and attachments", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    await api.dispose();
  });

  test("the templates gallery lists the built-in starters", async ({ page }) => {
    await page.goto("/templates");
    for (const name of ["Meeting notes", "Decision record", "How-to guide", "Retrospective"]) {
      await expect(page.getByText(name, { exact: true }).first()).toBeVisible();
    }
    await page.getByRole("button", { name: "Preview" }).first().click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("Escape");
  });

  test("use template from the gallery creates a page in a chosen space", async ({ page }) => {
    const title = uniq("From gallery");
    await page.goto("/templates");
    const card = page.getByRole("listitem").filter({ hasText: "Decision record" }).first();
    await card.getByRole("button", { name: "Use template" }).click();
    const dialog = page.getByRole("dialog", { name: /Use template: Decision record/ });
    await dialog.locator("select").first().selectOption(spaceKey());
    await dialog.getByLabel("Title").fill(title);
    await dialog.getByRole("button", { name: "Create page" }).click();
    await expect(page).toHaveURL(new RegExp(`/spaces/${spaceKey()}/pages/[0-9a-f-]+/edit$`));
    await expect(page.getByLabel("Page title")).toHaveValue(title);
  });

  test("a page can be created from a starter template in the new page dialog", async ({ page }) => {
    const title = uniq("Weekly sync");
    await page.goto(`/spaces/${spaceKey()}`);
    await page.getByRole("button", { name: "Create page", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Create page" });
    await dialog.getByLabel("Title").fill(title);
    await dialog.getByLabel("Template").selectOption({ label: "Meeting notes" });
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/edit$/);
    const surface = editorSurface(page);
    await expect(surface.locator("h2, h1, h3").first()).toBeVisible();
    await expect(surface.locator('div[data-type="panel"], table, ul').first()).toBeVisible();
  });

  test("a space template can be created and used", async ({ page }) => {
    const name = uniq("Incident review");
    await page.goto(`/spaces/${spaceKey()}/settings/templates`);
    await page.getByRole("button", { name: "New template" }).click();
    await page.getByLabel("Name").fill(name);
    await page.getByLabel("Description").fill("Space level template");
    await page.getByRole("textbox", { name: "Template content" }).click();
    await page.keyboard.type("Timeline of the incident");
    await page.getByRole("button", { name: "Save template" }).click();
    await expect(page.getByText(name)).toBeVisible();

    await page.goto(`/spaces/${spaceKey()}`);
    await page.getByRole("button", { name: "Create page", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Create page" });
    await dialog.getByLabel("Template").selectOption({ label: name });
    await dialog.getByLabel("Title").fill(uniq("Review"));
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(editorSurface(page)).toContainText("Timeline of the incident");
  });

  test("the space activity feed lists published pages", async ({ page }) => {
    const title = uniq("Feed Page");
    const api = await adminApi();
    await createPublishedPage(api, spaceKey(), title, "<p>feed</p>");
    await api.dispose();
    await page.goto(`/spaces/${spaceKey()}/activity`);
    await expect(page.getByRole("heading", { name: "Activity" })).toBeVisible();
    await expect(page.getByRole("main").getByText(title).first()).toBeVisible();
    await expect(page.getByRole("main").getByText(/published/).first()).toBeVisible();
  });

  test("attachments can be uploaded, downloaded and deleted", async ({ page }) => {
    const api = await adminApi();
    const p = await createPublishedPage(api, spaceKey(), uniq("Attach"), "<p>attachments</p>");
    await api.dispose();
    await page.goto(`/spaces/${spaceKey()}/pages/${p.id}/attachments`);
    await expect(page.getByRole("heading", { name: "Attachments" })).toBeVisible();
    await page.locator('input[type="file"]').setInputFiles(path.join(FIXTURE_DIR, "diagram.png"));
    await expect(page.getByText("diagram.png").first()).toBeVisible();
    await expect(page.getByText("image/png")).toBeVisible();

    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("link", { name: "Download" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("diagram.png");

    await page.getByRole("button", { name: "Delete diagram.png" }).click();
    await page.getByRole("dialog").getByRole("button", { name: /Delete/ }).click();
    await expect(page.getByText("image/png")).toHaveCount(0);
  });
});
