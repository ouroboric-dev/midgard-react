import { expect, test } from "@playwright/test";

test("the home page shows that the API answers", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Gallery" })).toBeVisible();
  await expect(page.getByText("API: ok")).toBeVisible();
});
