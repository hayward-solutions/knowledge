import fs from "node:fs";
import path from "node:path";
import { expect, test } from "@playwright/test";
import { ADMIN_STATE, FIXTURE_DIR } from "./support/env";
import { adminApi, createPublishedPage, ensureSpace, type ApiPage } from "./support/api";
import { spaceKey, uniq } from "./support/run";

test.use({ storageState: ADMIN_STATE });

const title = () => uniq("Export Me");

let exported: ApiPage;

test.describe.serial("export and import", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    exported = await createPublishedPage(
      api,
      spaceKey(),
      title(),
      "<h2>Overview</h2><p>This page is exported as <strong>Markdown</strong>.</p><ul><li>First item</li><li>Second item</li></ul>"
    );
    await api.dispose();
  });

  const pageUrl = () => `/spaces/${spaceKey()}/pages/${exported.id}/${exported.slug}`;

  test("export a page as Markdown", async ({ page }) => {
    await page.goto(pageUrl());
    await page.getByRole("button", { name: "More actions" }).click();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("menuitem", { name: /Markdown/ }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe(`${exported.slug}.md`);
    const file = await download.path();
    const text = fs.readFileSync(file, "utf8");
    expect(text).toContain(`# ${title()}`);
    expect(text).toContain("## Overview");
    expect(text).toContain("**Markdown**");
    expect(text).toMatch(/^[-*] First item/m);
  });

  test("export a page as standalone HTML", async ({ page }) => {
    await page.goto(pageUrl());
    await page.getByRole("button", { name: "More actions" }).click();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("menuitem", { name: /HTML/ }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe(`${exported.slug}.html`);
    const text = fs.readFileSync(await download.path(), "utf8");
    expect(text).toContain("<!doctype html>".toLowerCase());
    expect(text).toContain(title());
    expect(text).toContain("<strong>Markdown</strong>");
  });

  test("the space settings tabs link to every section", async ({ page }) => {
    const key = spaceKey();
    await page.goto(`/spaces/${key}/settings/general`);
    for (const [name, href] of [
      ["General", "general"],
      ["Members", "members"],
      ["Templates", "templates"],
      ["Import and export", "import-export"],
    ]) {
      await expect(page.getByRole("tab", { name, exact: true })).toHaveAttribute("href", `/spaces/${key}/settings/${href}`);
    }
    await page.getByRole("tab", { name: "Templates", exact: true }).click();
    await expect(page).toHaveURL(/settings\/templates$/);
    await expect(page.getByRole("tab", { name: "Import and export", exact: true })).toBeVisible();
    await page.getByRole("tab", { name: "Import and export", exact: true }).click();
    await expect(page).toHaveURL(/settings\/import-export$/);
    await expect(page.getByRole("tab", { name: "General", exact: true })).toBeVisible();
  });

  test("export the whole space as a zip", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}/settings/import-export`);
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("link", { name: "Export space" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe(`${spaceKey()}-export.zip`);
    const bytes = fs.readFileSync(await download.path());
    expect(bytes.subarray(0, 2).toString("latin1")).toBe("PK");
    expect(bytes.toString("latin1")).toContain(".md");
  });

  test("import a Markdown file as a new page", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}/settings/import-export`);
    await page.getByRole("button", { name: "Import Markdown" }).click();
    const dialog = page.getByRole("dialog", { name: "Import Markdown" });
    await dialog.locator('input[type="file"][accept*=".md"]').setInputFiles(path.join(FIXTURE_DIR, "import-notes.md"));
    await expect(dialog.getByText("1 file selected")).toBeVisible();
    await dialog.getByRole("button", { name: /^Import/ }).click();
    await expect(dialog.getByText("Created 1 page")).toBeVisible();
    await dialog.getByRole("link", { name: "Imported runbook" }).click();
    await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/imported-runbook$/);
    const content = page.locator(".kn-content").first();
    await expect(page.getByRole("heading", { level: 1, name: "Imported runbook" })).toBeVisible();
    await expect(content).toContainText("Check the dashboards");
    await expect(content.locator("table")).toContainText("core");
    await expect(page.getByRole("tree").getByRole("link", { name: "Imported runbook" })).toBeVisible();
  });
});
