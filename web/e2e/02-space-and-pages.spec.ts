import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { ADMIN_STATE, FIXTURE_DIR } from "./support/env";
import { editorSurface, focusEditorEnd, publishFromEditor, slash, waitForSaved } from "./support/editor";
import { adminApi, createPublishedPage, getSpace } from "./support/api";
import { spaceKey, uniq } from "./support/run";

test.use({ storageState: ADMIN_STATE });

const pageTitle = () => uniq("Runbook");

let pageUrl = "";
let pageId = "";

async function openMoreActions(page: Page, item: string) {
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: item }).click();
}

test.describe.serial("space and page lifecycle", () => {
  test("create the ENG space from the directory", async ({ page }) => {
    const key = spaceKey();
    const api = await adminApi();
    const existing = await getSpace(api, key);
    await api.dispose();
    await page.goto("/spaces");
    if (existing) {
      await expect(page.getByRole("link", { name: new RegExp(key) }).first()).toBeVisible();
      return;
    }
    await page.getByRole("button", { name: "Create space" }).first().click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Name").fill("Engineering");
    await dialog.getByLabel("Key").fill(key);
    await dialog.getByLabel("Description").fill("Engineering docs for the e2e run");
    await dialog.getByRole("button", { name: "Create space" }).click();
    await expect(page).toHaveURL(new RegExp(`/spaces/${key}$`));
  });

  test("space home page renders with the sidebar tree", async ({ page }) => {
    const key = spaceKey();
    await page.goto(`/spaces/${key}`);
    await expect(page.getByRole("heading", { level: 1, name: "Engineering" })).toBeVisible();
    await expect(page.getByRole("tree")).toBeVisible();
    await expect(page.getByRole("treeitem", { name: /Engineering/ })).toBeVisible();
    await page.goto("/spaces");
    await expect(page.getByRole("link", { name: new RegExp(`Engineering`) }).first()).toBeVisible();
  });

  test("top bar Create opens the new page dialog inside a space", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}`);
    await page.getByRole("button", { name: "Create", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Create page" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel("Template")).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);

    await page.goto("/spaces");
    await page.getByRole("link", { name: "Create", exact: true }).click();
    await expect(page).toHaveURL(/\/spaces$/);
  });

  test("create a page from the sidebar dialog", async ({ page }) => {
    const key = spaceKey();
    await page.goto(`/spaces/${key}`);
    await page.getByRole("button", { name: "Create page", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Create page" });
    await dialog.getByLabel("Title").fill(pageTitle());
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/edit$/);
    pageId = page.url().match(/pages\/([0-9a-f-]+)\/edit/)![1];
    await expect(page.getByLabel("Page title")).toHaveValue(pageTitle());
  });

  test("edit with text, a panel, an image and a table, then publish", async ({ page }) => {
    const key = spaceKey();
    await page.goto(`/spaces/${key}/pages/${pageId}/edit`);
    await expect(page.getByLabel("Page title")).toHaveValue(pageTitle());

    const surface = editorSurface(page);
    await surface.click();
    await page.keyboard.type("Alpha paragraph for the runbook.");
    await page.keyboard.press("Enter");

    await slash(page, "info");
    await page.keyboard.type("Heads up from the panel");
    await page.keyboard.press("Enter");
    await page.keyboard.press("Enter");

    await page.locator('input[type="file"][accept="image/*"]').setInputFiles(path.join(FIXTURE_DIR, "diagram.png"));
    await expect(surface.locator("img")).toHaveCount(1, { timeout: 20_000 });
    await expect
      .poll(() => surface.locator("img").evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0))
      .toBe(true);

    await focusEditorEnd(page);
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("Enter");
    await slash(page, "table");
    await page.keyboard.type("Name");
    await page.keyboard.press("Tab");
    await page.keyboard.type("Role");
    await page.keyboard.press("Tab");
    await page.keyboard.type("Team");
    await page.keyboard.press("Tab");
    await page.keyboard.type("Ada");

    await expect(surface.locator('div[data-type="panel"]')).toContainText("Heads up from the panel");
    await expect(surface.locator("table")).toBeVisible();
    await waitForSaved(page);

    await publishFromEditor(page, "Initial draft");
    pageUrl = page.url();

    const content = page.locator(".kn-content").first();
    await expect(page.getByRole("heading", { level: 1, name: pageTitle() })).toBeVisible();
    await expect(content).toContainText("Alpha paragraph for the runbook.");
    await expect(content.locator('div[data-type="panel"]')).toContainText("Heads up from the panel");
    await expect(content.locator("table th").first()).toHaveText("Name");
    await expect
      .poll(() => content.locator("img").first().evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0))
      .toBe(true);
    await expect(page.getByRole("link", { name: "Version 1" })).toBeVisible();
  });

  test("the editor produces HTML the sanitiser keeps intact", async ({ page }) => {
    const api = await adminApi();
    const res = await api.get(`pages/${pageId}`);
    const detail = await res.json();
    expect(detail.content_html).toContain('data-type="panel"');
    expect(detail.content_html).toContain("<table");
    expect(detail.content_html).toMatch(/<img[^>]+data-attachment-id=/);
    await api.dispose();
  });

  test("edit again and publish a second version", async ({ page }) => {
    await page.goto(pageUrl);
    await page.getByRole("link", { name: "Edit", exact: true }).click();
    await expect(page).toHaveURL(/\/edit$/);
    const surface = editorSurface(page);
    await expect(surface).toContainText("Alpha paragraph for the runbook.");
    await surface.locator("p").first().click();
    await page.keyboard.press("End");
    await page.keyboard.type(" Second revision added.");
    await waitForSaved(page);
    await publishFromEditor(page, "Second revision");
    await expect(page.locator(".kn-content").first()).toContainText("Second revision added.");
    await expect(page.getByRole("link", { name: "Version 2" })).toBeVisible();
  });

  test("history lists both versions with a diff and restores version 1", async ({ page }) => {
    await page.goto(pageUrl);
    await openMoreActions(page, "History");
    await expect(page).toHaveURL(/\/history$/);
    await expect(page.getByRole("heading", { name: "Version history" })).toBeVisible();

    const versions = page.getByRole("region", { name: "Versions" });
    await expect(versions.getByText("Version 2", { exact: true })).toBeVisible();
    await expect(versions.getByText("Version 1", { exact: true })).toBeVisible();
    await expect(versions.getByText("Current")).toBeVisible();
    await expect(versions.getByText("Initial draft")).toBeVisible();
    await expect(versions.getByText("Second revision", { exact: true })).toBeVisible();

    const comparison = page.getByRole("region", { name: "Comparison" });
    await expect(comparison.getByText(/Comparing/)).toBeVisible();
    await expect(comparison.locator("ins").first()).toContainText("Second");

    await page.getByRole("tab", { name: "Side by side" }).click();
    await expect(comparison.getByText("Version 1").first()).toBeVisible();

    await versions.getByRole("button", { name: "Restore" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Restore" }).click();
    await expect(page).toHaveURL(/\/spaces\/[A-Z0-9]+\/pages\/[0-9a-f-]+\/[^/]+$/);
    await expect(page.locator(".kn-content").first()).toContainText("Alpha paragraph for the runbook.");
    await expect(page.locator(".kn-content").first()).not.toContainText("Second revision added.");
    await expect(page.getByRole("link", { name: "Version 3" })).toBeVisible();

    await openMoreActions(page, "History");
    await expect(page.getByRole("region", { name: "Versions" }).getByText("Restored version 1")).toBeVisible();
  });

  test("pages titled Edit or History get reserved-word safe slugs", async ({ page }) => {
    const key = spaceKey();
    const api = await adminApi();
    const created = await createPublishedPage(api, key, "History", "<p>History page body</p>");
    await api.dispose();
    expect(created.slug).toBe("history-page");
    await page.goto(`/spaces/${key}`);
    await page.getByRole("tree").getByRole("link", { name: "History", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/pages/${created.id}/history-page$`));
    await expect(page.getByRole("heading", { level: 1, name: "History" })).toBeVisible();
    await expect(page.getByText("History page body")).toBeVisible();
  });
});
