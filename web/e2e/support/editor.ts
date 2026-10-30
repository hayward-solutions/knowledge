import { expect, type Locator, type Page } from "@playwright/test";

export function editorSurface(page: Page): Locator {
  return page.getByRole("textbox", { name: "Page content" });
}

export async function focusEditorEnd(page: Page) {
  const surface = editorSurface(page);
  await expect(surface).toBeVisible();
  await surface.click();
  await page.keyboard.press("ControlOrMeta+End");
}

export async function slash(page: Page, command: string) {
  await page.keyboard.type(`/${command}`);
  await expect(page.locator(".kn-suggest")).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.locator(".kn-suggest")).toHaveCount(0);
}

export async function publishFromEditor(page: Page, message?: string) {
  await page.getByRole("button", { name: "Publish", exact: true }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  if (message) await dialog.getByPlaceholder("What changed?").fill(message);
  await dialog.getByRole("button", { name: "Publish", exact: true }).click();
  await expect(page).toHaveURL(/\/spaces\/[A-Z0-9]+\/pages\/[0-9a-f-]+\/(?!edit)[^/]+$/);
}

export async function waitForSaved(page: Page) {
  await expect(page.getByText(/^Saved/)).toBeVisible({ timeout: 15_000 });
}
