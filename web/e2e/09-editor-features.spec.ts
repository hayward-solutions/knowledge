import { expect, test } from "@playwright/test";
import { ADMIN_STATE } from "./support/env";
import { adminApi, addMember, createPublishedPage, createUser, ensureSpace } from "./support/api";
import { editorSurface, publishFromEditor, slash, waitForSaved } from "./support/editor";
import { run, spaceKey, uniq } from "./support/run";
import { uiLogin } from "./support/ui";

test.use({ storageState: ADMIN_STATE });

async function newDraftPage(title: string): Promise<string> {
  const api = await adminApi();
  const res = await api.post(`spaces/${spaceKey()}/pages`, { data: { title } });
  const created = (await res.json()) as { id: string };
  await api.dispose();
  return created.id;
}

const blocks: { command: string; selector: string }[] = [
  { command: "h1", selector: "h1" },
  { command: "h2", selector: "h2" },
  { command: "h3", selector: "h3" },
  { command: "bulleted", selector: "ul:not([data-type='taskList'])" },
  { command: "numbered", selector: "ol" },
  { command: "task", selector: "ul[data-type='taskList'] li" },
  { command: "code", selector: "pre code" },
  { command: "info", selector: "div[data-type='panel'][data-color='info']" },
  { command: "note", selector: "div[data-type='panel'][data-color='note']" },
  { command: "success", selector: "div[data-type='panel'][data-color='success']" },
  { command: "warning", selector: "div[data-type='panel'][data-color='warning']" },
  { command: "error", selector: "div[data-type='panel'][data-color='error']" },
  { command: "expand", selector: "[data-type='expand']" },
  { command: "status", selector: "[data-type='status']" },
  { command: "toc", selector: "[data-type='toc']" },
  { command: "divider", selector: "hr" },
  { command: "quote", selector: "blockquote" },
  { command: "table", selector: "table" },
];

test.describe.serial("editor features", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    await api.dispose();
  });

  test("every slash menu block inserts the right element", async ({ page }) => {
    const id = await newDraftPage(uniq("Slash tour"));
    await page.goto(`/spaces/${spaceKey()}/pages/${id}/edit`);
    const surface = editorSurface(page);
    await surface.click();
    const clear = async () => {
      await page.keyboard.press("ControlOrMeta+a");
      await page.keyboard.press("Backspace");
      await page.keyboard.press("ControlOrMeta+Alt+0");
    };

    await slash(page, "date");
    await expect(surface).toContainText(String(new Date().getFullYear()));
    await clear();

    for (const trigger of ["mention", "wiki"]) {
      await page.keyboard.type(`/${trigger}`);
      await expect(page.locator(".kn-suggest")).toBeVisible();
      await page.keyboard.press("Enter");
      await expect(page.locator(".kn-suggest")).toBeVisible();
      await page.keyboard.press("Escape");
      await clear();
    }

    for (const block of blocks) {
      await slash(page, block.command);
      await expect(surface.locator(block.selector).first(), `/${block.command}`).toBeVisible();
      if (block.command === "table") break;
      await clear();
      await expect(surface.locator(block.selector)).toHaveCount(0);
    }
  });

  test("mention, page link, task list and code block survive publishing", async ({ page }) => {
    const target = uniq("Link Target");
    const api = await adminApi();
    const linked = await createPublishedPage(api, spaceKey(), target, "<p>Target body</p>");
    await api.dispose();

    const title = uniq("Rich content");
    const id = await newDraftPage(title);
    await page.goto(`/spaces/${spaceKey()}/pages/${id}/edit`);
    const surface = editorSurface(page);
    await surface.click();

    await page.keyboard.type("Ping @Adm");
    await expect(page.getByRole("option", { name: /Admin/ })).toBeVisible();
    await page.keyboard.press("Enter");
    await page.keyboard.type(" and see [[");
    await page.keyboard.type(target);
    await expect(page.getByRole("option", { name: new RegExp(target) })).toBeVisible();
    await page.keyboard.press("Enter");
    await page.keyboard.press("Enter");

    await slash(page, "task");
    await page.keyboard.type("Write the docs");
    await page.keyboard.press("Enter");
    await page.keyboard.press("Enter");
    await slash(page, "code");
    await page.keyboard.type("const answer = 42;");

    await expect(surface.locator("[data-type='mention']")).toContainText("@Admin");
    await expect(surface.locator("a[data-type='page-link']")).toContainText(target);
    await waitForSaved(page);
    await publishFromEditor(page, "Rich content");

    const content = page.locator(".kn-content").first();
    await expect(content.locator("[data-type='mention']")).toContainText("@Admin");
    await expect(content.locator("a[data-type='page-link']")).toHaveAttribute("href", new RegExp(`/pages/${linked.id}/`));
    await expect(content.locator("ul[data-type='taskList'] li")).toContainText("Write the docs");
    await expect(content.locator("pre code")).toContainText("const answer = 42;");

    await content.locator("a[data-type='page-link']").click();
    await expect(page).toHaveURL(new RegExp(`/pages/${linked.id}/`));
    await expect(page.getByRole("heading", { level: 1, name: target })).toBeVisible();
  });

  test("an unpublished draft is autosaved and resumed", async ({ page }) => {
    const title = uniq("Draft resume");
    const id = await newDraftPage(title);
    await page.goto(`/spaces/${spaceKey()}/pages/${id}/edit`);
    await editorSurface(page).click();
    await page.keyboard.type("Work in progress text");
    await waitForSaved(page);
    await page.getByRole("button", { name: "Close" }).click();

    await page.goto(`/spaces/${spaceKey()}/pages/${id}/edit`);
    await expect(editorSurface(page)).toContainText("Work in progress text");
    await expect(page.getByRole("status").filter({ hasText: "Resuming your unpublished changes" })).toBeVisible();
    await page.getByRole("button", { name: "Discard draft" }).first().click();
    await page.getByRole("dialog").getByRole("button", { name: "Discard draft" }).click();
    await expect(editorSurface(page)).not.toContainText("Work in progress text");
  });

  test("presence banner and publish conflict between two editors", async ({ browser, page }) => {
    const second = {
      name: `Ada Lovelace ${run().runId}`,
      email: `ada.${run().runId}@example.com`,
      password: "Ada-Lovelace-Pass-1!",
    };
    const api = await adminApi();
    const user = await createUser(api, second.email, second.name, second.password);
    await addMember(api, spaceKey(), user.id, "editor");
    const shared = await createPublishedPage(api, spaceKey(), uniq("Shared page"), "<p>Shared base text.</p>");
    await api.dispose();
    const editUrl = `/spaces/${spaceKey()}/pages/${shared.id}/edit`;

    const ctx = await browser.newContext();
    const other = await ctx.newPage();
    await uiLogin(other, second.email, second.password);
    await other.goto(editUrl);
    const otherSurface = editorSurface(other);
    await expect(otherSurface).toContainText("Shared base text.");

    await page.goto(editUrl);
    const surface = editorSurface(page);
    await expect(surface).toContainText("Shared base text.");
    await expect(page.getByRole("status").filter({ hasText: `${second.name} is also editing this page` })).toBeVisible({
      timeout: 20_000,
    });

    await otherSurface.locator("p").first().click();
    await other.keyboard.press("End");
    await other.keyboard.type(" Ada was here.");
    await waitForSaved(other);
    await publishFromEditor(other, "Ada update");

    await surface.locator("p").first().click();
    await page.keyboard.press("End");
    await page.keyboard.type(" Admin was here.");
    await waitForSaved(page);
    await page.getByRole("button", { name: "Publish", exact: true }).first().click();
    await page.getByRole("dialog").getByRole("button", { name: "Publish", exact: true }).click();
    const conflict = page.getByRole("dialog", { name: "This page was published by someone else" });
    await expect(conflict).toBeVisible();
    await conflict.getByRole("button", { name: "Publish anyway" }).click();
    await expect(page).toHaveURL(new RegExp(`/pages/${shared.id}/[^/]+$`));
    await expect(page.locator(".kn-content").first()).toContainText("Admin was here.");
    await expect(page.getByRole("link", { name: "Version 3" })).toBeVisible();
    await ctx.close();
  });
});
