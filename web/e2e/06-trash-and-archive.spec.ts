import { expect, test } from "@playwright/test";
import { ADMIN_STATE } from "./support/env";
import { adminApi, createPublishedPage, createUser, ensureSpace, type ApiPage } from "./support/api";
import { run, spaceKey, uniq } from "./support/run";
import { uiLogin } from "./support/ui";

test.use({ storageState: ADMIN_STATE });

const parentTitle = () => uniq("Trash Parent");
const childTitle = () => uniq("Trash Child");
const archiveKey = () => `AR${run().runId.toUpperCase().slice(-6)}`;

let parent: ApiPage;
let child: ApiPage;

test.describe.serial("trash", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    parent = await createPublishedPage(api, spaceKey(), parentTitle(), "<p>Parent body</p>");
    child = await createPublishedPage(api, spaceKey(), childTitle(), "<p>Child body</p>", parent.id);
    await api.dispose();
  });

  const parentUrl = () => `/spaces/${spaceKey()}/pages/${parent.id}/${parent.slug}`;

  test("trash a page and its subtree from the page menu", async ({ page }) => {
    await page.goto(parentUrl());
    const tree = page.getByRole("tree");
    await expect(tree.getByRole("link", { name: parentTitle() })).toBeVisible();
    await page.getByRole("button", { name: "More actions" }).click();
    await page.getByRole("menuitem", { name: "Move to trash" }).click();
    await page.getByRole("dialog", { name: "Move to trash" }).getByRole("button", { name: "Move to trash" }).click();
    await expect(tree.getByRole("link", { name: parentTitle() })).toHaveCount(0);
    await page.goto(`/spaces/${spaceKey()}`);
    await expect(page.getByRole("tree").getByRole("link", { name: childTitle() })).toHaveCount(0);
  });

  test("the trash view lists it and restore brings the subtree back", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}/trash`);
    const row = page.getByRole("listitem").filter({ hasText: parentTitle() });
    await expect(row).toBeVisible();
    await expect(row.getByText("Has subpages")).toBeVisible();
    await row.getByRole("button", { name: "Restore" }).click();
    await expect(row).toHaveCount(0);
    await page.goto(parentUrl());
    await expect(page.getByRole("heading", { level: 1, name: parentTitle() })).toBeVisible();
    await expect(page.getByRole("tree").getByRole("link", { name: parentTitle() })).toBeVisible();
    await page.getByRole("tree").getByRole("treeitem", { name: new RegExp(parentTitle()) }).getByRole("button", { name: /Expand|Collapse/ }).first().click().catch(() => undefined);
    await expect(page.getByRole("main").getByRole("link", { name: childTitle() })).toBeVisible();
  });

  test("a trashed page can be deleted permanently", async ({ page }) => {
    await page.goto(parentUrl());
    await page.getByRole("button", { name: "More actions" }).click();
    await page.getByRole("menuitem", { name: "Move to trash" }).click();
    await page.getByRole("dialog", { name: "Move to trash" }).getByRole("button", { name: "Move to trash" }).click();

    await page.goto(`/spaces/${spaceKey()}/trash`);
    const row = page.getByRole("listitem").filter({ hasText: parentTitle() });
    await row.getByRole("button", { name: "Delete permanently" }).click();
    await page.getByRole("dialog", { name: "Delete permanently" }).getByRole("button", { name: "Delete permanently" }).click();
    await expect(row).toHaveCount(0);

    const api = await adminApi();
    const res = await api.get(`pages/${parent.id}`);
    expect(res.status()).toBe(404);
    await api.dispose();
  });
});

test("a page trashed elsewhere updates in place without crashing the comment composer", async ({ page, context }) => {
  const api = await adminApi();
  await ensureSpace(api, spaceKey());
  const live = await createPublishedPage(api, spaceKey(), uniq("Trashed elsewhere"), "<p>Body</p>");
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));

  await page.goto(`/spaces/${spaceKey()}/pages/${live.id}/${live.slug}`);
  await expect(page.getByRole("textbox", { name: "Comment", exact: true })).toBeVisible();

  const res = await api.post(`pages/${live.id}/trash`);
  expect(res.ok()).toBeTruthy();
  await api.dispose();

  await context.setOffline(true);
  await context.setOffline(false);
  await expect(page.getByText("Trashed", { exact: true })).toBeVisible();
  await expect(page.getByText("Comments are read-only while the page is in the trash.")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Comment", exact: true })).toHaveAttribute("contenteditable", "false");
  await expect(page.getByRole("heading", { level: 1, name: /Trashed elsewhere/ })).toBeVisible();
  expect(errors).toEqual([]);
});

test.describe.serial("archive", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, archiveKey(), "Archive candidate");
    await api.dispose();
  });

  test("archiving a space makes it read-only and unarchiving restores editing", async ({ page }) => {
    const key = archiveKey();
    await page.goto(`/spaces/${key}/settings/general`);
    await page.getByRole("button", { name: "Archive space" }).click();
    await page.getByRole("dialog", { name: "Archive space" }).getByRole("button", { name: "Archive", exact: true }).click();
    await expect(page.getByText("This space is archived and read-only.")).toBeVisible();

    await page.goto(`/spaces/${key}`);
    await expect(page.getByText("This space is archived and read-only.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Create page", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Edit" })).toBeDisabled();

    await page.goto("/spaces");
    await expect(page.getByRole("link", { name: new RegExp(key) }).first()).toBeVisible();

    await page.goto(`/spaces/${key}/settings/general`);
    await page.getByRole("button", { name: "Unarchive space" }).click();
    await expect(page.getByText("This space is archived and read-only.")).toHaveCount(0);
    await page.goto(`/spaces/${key}`);
    await expect(page.getByRole("button", { name: "Create page", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Edit" })).toBeVisible();
  });
});

test.describe.serial("move and copy", () => {
  let target: ApiPage;
  let mover: ApiPage;
  const targetTitle = () => uniq("Move Target");
  const moverTitle = () => uniq("Move Me");

  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    target = await createPublishedPage(api, spaceKey(), targetTitle(), "<p>Target</p>");
    mover = await createPublishedPage(api, spaceKey(), moverTitle(), "<p>Mover</p>");
    await api.dispose();
  });

  test("move a page under another page with the move dialog", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}/pages/${mover.id}/${mover.slug}`);
    await page.getByRole("button", { name: "More actions" }).click();
    await page.getByRole("menuitem", { name: "Move", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: /^Move/ });
    await dialog.getByPlaceholder("Filter pages").fill(targetTitle());
    await dialog.getByRole("radiogroup").getByText(targetTitle()).click();
    await dialog.getByRole("button", { name: "Move", exact: true }).click();
    await expect(page.getByText("Page moved")).toBeVisible();
    await page.reload();
    const crumbs = page.getByRole("navigation", { name: "Breadcrumb" });
    await expect(crumbs.getByRole("link", { name: targetTitle() })).toBeVisible();

    const api = await adminApi();
    const detail = (await (await api.get(`pages/${mover.id}`)).json()) as { parent_id: string };
    expect(detail.parent_id).toBe(target.id);
    await api.dispose();

    await page.goto(`/spaces/${spaceKey()}/pages/${target.id}/${target.slug}`);
    await expect(page.getByRole("main").getByRole("link", { name: moverTitle() })).toBeVisible();
  });

  test("drag a page onto another page in the sidebar tree", async ({ page }) => {
    const api = await adminApi();
    const dragged = await createPublishedPage(api, spaceKey(), uniq("Drag Me"), "<p>Drag</p>");
    const dropTarget = await createPublishedPage(api, spaceKey(), uniq("Drop Here"), "<p>Drop</p>");
    await page.goto(`/spaces/${spaceKey()}`);
    const tree = page.getByRole("tree");
    const source = tree.getByRole("treeitem", { name: uniq("Drag Me"), exact: true });
    const destination = tree.getByRole("treeitem", { name: uniq("Drop Here"), exact: true });
    await expect(source).toBeVisible();
    await expect(destination).toBeVisible();
    await destination.scrollIntoViewIfNeeded();
    await source.scrollIntoViewIfNeeded();
    const from = (await source.boundingBox())!;
    const to = (await destination.boundingBox())!;
    await page.mouse.move(from.x + 40, from.y + from.height / 2);
    await page.mouse.down();
    await page.mouse.move(from.x + 40, from.y + from.height / 2 + 4, { steps: 4 });
    await page.mouse.move(to.x + 40, to.y + to.height / 2, { steps: 12 });
    await page.mouse.up();
    await expect
      .poll(async () => ((await (await api.get(`pages/${dragged.id}`)).json()) as { parent_id: string }).parent_id)
      .toBe(dropTarget.id);
    await api.dispose();
  });

  test("copy a page creates an unpublished draft copy", async ({ page }) => {
    await page.goto(`/spaces/${spaceKey()}/pages/${target.id}/${target.slug}`);
    await page.getByRole("button", { name: "More actions" }).click();
    await page.getByRole("menuitem", { name: "Copy" }).click();
    const dialog = page.getByRole("dialog", { name: "Copy page" });
    await expect(dialog.getByLabel("Title")).toHaveValue(`Copy of ${targetTitle()}`);
    await dialog.getByRole("button", { name: "Copy", exact: true }).click();
    await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/(edit|[^/]+)$/);
    await expect(page.getByRole("tree").getByRole("link", { name: `Copy of ${targetTitle()}` })).toBeVisible();
  });
});

test.describe("viewer permissions", () => {
  test("a viewer of an internal space can read but not edit", async ({ browser }) => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    const readOnly = await createPublishedPage(api, spaceKey(), uniq("Read Only"), "<p>Read only body</p>");
    const email = `viewer.${run().runId}@example.com`;
    const password = "Viewer-Only-Pass-1!";
    await createUser(api, email, "Vera Viewer", password);
    await api.dispose();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await uiLogin(page, email, password);
    await page.goto(`/spaces/${spaceKey()}/pages/${readOnly.id}/${readOnly.slug}`);
    await expect(page.locator(".kn-content").first()).toContainText("Read only body");
    await expect(page.getByRole("link", { name: "Edit", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Create page", exact: true })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Trash" })).toHaveCount(0);

    await page.goto(`/spaces/${spaceKey()}/pages/${readOnly.id}/edit`);
    await expect(page.getByRole("button", { name: "Publish", exact: true })).toHaveCount(0);
    await ctx.close();
  });
});

test.describe("private spaces and small screens", () => {
  test("a private space is hidden from non-members", async ({ browser }) => {
    const api = await adminApi();
    const key = `PV${run().runId.toUpperCase().slice(-6)}`;
    const created = await api.post("spaces", {
      data: { key, name: "Private notes", description: "", visibility: "private" },
    });
    expect(created.ok()).toBeTruthy();
    const email = `outsider.${run().runId}@example.com`;
    const password = "Outsider-Only-Pass-1!";
    await createUser(api, email, "Olga Outsider", password);
    await api.dispose();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await uiLogin(page, email, password);
    await page.goto(`/spaces/${key}`);
    await expect(page.getByText("Space not found")).toBeVisible();
    await page.goto("/spaces");
    await expect(page.getByRole("heading", { name: "Spaces" })).toBeVisible();
    await expect(page.getByText("Private notes")).toHaveCount(0);
    await ctx.close();
  });

  test("the page view works at phone width without horizontal scrolling", async ({ page }) => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    const p = await createPublishedPage(api, spaceKey(), uniq("Phone view"), "<p>Phone body</p>");
    await api.dispose();
    await page.setViewportSize({ width: 390, height: 800 });
    await page.goto(`/spaces/${spaceKey()}/pages/${p.id}/${p.slug}`);
    await expect(page.getByRole("heading", { level: 1, name: /Phone view/ })).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    await page.getByRole("button", { name: "Open navigation" }).click();
    await expect(page.getByRole("tree")).toBeVisible();
    await page.getByRole("button", { name: "Close navigation" }).first().click();
  });
});
