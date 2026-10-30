import { expect, test } from "@playwright/test";
import { ADMIN_STATE } from "./support/env";
import { adminApi, createPublishedPage, ensureSpace, type ApiPage } from "./support/api";
import { run, spaceKey, uniq } from "./support/run";

test.use({ storageState: ADMIN_STATE });

const marker = () => `zephyr${run().runId}`;
const title = () => uniq("Deploy Guide");

let guide: ApiPage;

test.describe.serial("comments, labels, search and starring", () => {
  test.beforeAll(async () => {
    const api = await adminApi();
    await ensureSpace(api, spaceKey());
    guide = await createPublishedPage(
      api,
      spaceKey(),
      title(),
      `<p>The ${marker()} deployment strategy covers canary rollouts.</p><p>Rollbacks are automatic when health checks fail.</p>`
    );
    await api.dispose();
  });

  const guideUrl = () => `/spaces/${spaceKey()}/pages/${guide.id}/${guide.slug}`;

  test("add a page comment and reply to it", async ({ page }) => {
    await page.goto(guideUrl());
    const composer = page.getByRole("textbox", { name: "Comment", exact: true });
    await composer.click();
    await page.keyboard.type("Looks good to me, ship it.");
    await page.getByRole("button", { name: "Comment", exact: true }).click();
    const comments = page.locator("section#comments");
    await expect(comments).toContainText("Looks good to me, ship it.");
    await expect(comments.getByRole("heading", { name: "Comments (1)" })).toBeVisible();

    await comments.getByRole("button", { name: "Reply" }).first().click();
    await page.getByRole("textbox", { name: "Reply" }).click();
    await page.keyboard.type("Thanks, merging now.");
    await page.getByRole("button", { name: "Reply", exact: true }).last().click();
    await expect(comments).toContainText("Thanks, merging now.");
    await expect(comments.getByRole("heading", { name: "Comments (2)" })).toBeVisible();
  });

  test("select text and leave an inline comment, then resolve and reopen it", async ({ page }) => {
    await page.goto(guideUrl());
    const first = page.locator(".kn-content p").first();
    await expect(first).toContainText(marker());
    await first.click({ clickCount: 3 });
    const floating = page.locator("[data-selection-comment]");
    await expect(floating).toBeVisible();
    await floating.click();

    const dialog = page.getByRole("dialog", { name: "Add inline comment" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("textbox", { name: "Inline comment" }).click();
    await page.keyboard.type("Is canary the right word here?");
    await dialog.getByRole("button", { name: "Comment", exact: true }).click();
    await expect(dialog).toHaveCount(0);

    const inline = page.getByRole("complementary", { name: "Inline comments" });
    await expect(inline).toContainText("Is canary the right word here?");
    await expect(page.locator(".kn-content span[data-type='comment']").first()).toBeVisible();

    await inline.getByRole("button", { name: "Resolve" }).click();
    await expect(inline.getByRole("button", { name: "Reopen" })).toBeVisible();
    await expect(inline).toContainText("Resolved");

    await inline.getByRole("button", { name: "Reopen" }).click();
    await expect(inline.getByRole("button", { name: "Resolve" })).toBeVisible();
  });

  test("labels can be added, shown on the page and browsed", async ({ page }) => {
    const label = `e2e-${run().runId}`;
    await page.goto(guideUrl());
    await page.getByRole("button", { name: "Add label" }).click();
    const picker = page.getByRole("combobox", { name: "Label" });
    await picker.fill(label);
    await picker.press("Enter");
    const chip = page.getByRole("link", { name: label, exact: true });
    await expect(chip).toBeVisible();

    await page.reload();
    await expect(page.getByRole("link", { name: label, exact: true })).toBeVisible();

    await page.getByRole("link", { name: label, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/spaces/${spaceKey()}/labels/${label}$`));
    await expect(page.getByRole("heading", { level: 1, name: label })).toBeVisible();
    await expect(page.getByRole("main").getByRole("link", { name: title() })).toBeVisible();
  });

  test("search finds the page with a highlighted snippet", async ({ page }) => {
    await page.goto(`/search?q=${marker()}`);
    const hit = page.getByRole("listitem").filter({ hasText: title() });
    await expect(hit).toBeVisible();
    await expect(hit.locator("mark").first()).toContainText(marker(), { ignoreCase: true });
    await hit.getByRole("link", { name: title() }).click();
    await expect(page).toHaveURL(new RegExp(`/pages/${guide.id}/`));
  });

  test("search filters by label and space", async ({ page }) => {
    const label = `e2e-${run().runId}`;
    await page.goto(`/search?q=${marker()}&space=${spaceKey()}&label=${label}`);
    await expect(page.getByRole("listitem").filter({ hasText: title() })).toBeVisible();
    await page.goto(`/search?q=${marker()}&label=no-such-label-${run().runId}`);
    await expect(page.getByText("No results")).toBeVisible();
  });

  test("quick search suggests pages by title and opens one", async ({ page }) => {
    await page.goto("/");
    const box = page.getByRole("combobox", { name: "Search" });
    await box.fill(title());
    const option = page.getByRole("option", { name: new RegExp(`^${title()}`) });
    await expect(option).toBeVisible();
    await option.click();
    await expect(page).toHaveURL(new RegExp(`/pages/${guide.id}/`));
  });

  test("the slash key focuses the quick search", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("/");
    await expect(page.getByRole("combobox", { name: "Search" })).toBeFocused();
  });

  test("starring a page shows it on the dashboard", async ({ page }) => {
    await page.goto(guideUrl());
    await page.getByRole("button", { name: "Star this page" }).click();
    await expect(page.getByRole("button", { name: "Remove star" })).toBeVisible();
    await page.goto("/");
    const starred = page.getByRole("region", { name: "Starred" });
    await expect(starred.getByRole("link", { name: title() })).toBeVisible();
    await expect(page.getByRole("region", { name: "Recently viewed" }).getByRole("link", { name: title() })).toBeVisible();

    await page.goto(guideUrl());
    await page.getByRole("button", { name: "Remove star" }).click();
    await expect(page.getByRole("button", { name: "Star this page" })).toBeVisible();
    await page.goto("/");
    await expect(page.getByRole("region", { name: "Starred" }).getByRole("link", { name: title() })).toHaveCount(0);
  });
});
