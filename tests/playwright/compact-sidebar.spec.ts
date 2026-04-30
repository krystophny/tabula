import { expect, test, type Page } from '@playwright/test';

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._slopshellApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    const wsOpen = (window as any).WebSocket.OPEN;
    return s.chatWs?.readyState === wsOpen && s.canvasWs?.readyState === wsOpen;
  }, null, { timeout: 8_000 });
}

async function seedSectionFixture(
  page: Page,
  counts: {
    projectItemsOpen?: number;
    peopleOpen?: number;
    driftReview?: number;
    dedupReview?: number;
    recentMeetings?: number;
  },
) {
  await page.evaluate((data) => {
    (window as any).__itemSidebarSectionCounts = {
      project_items_open: data.projectItemsOpen ?? 0,
      people_open: data.peopleOpen ?? 0,
      drift_review: data.driftReview ?? 0,
      dedup_review: data.dedupReview ?? 0,
      recent_meetings: data.recentMeetings ?? 0,
    };
  }, counts);
}

async function openInbox(page: Page) {
  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  await page.locator('.sidebar-tab', { hasText: 'Inbox' }).click();
  await expect(page.locator('.sidebar-tab.is-active')).toContainText('Inbox');
}

test.describe('compact sidebar navigation (#746)', () => {
  test('exposes a single primary header with sphere selector, workspace pin and capture button above queue tabs', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await openInbox(page);

    const primary = page.locator('#sidebar-primary');
    await expect(primary).toBeVisible();
    const sphereButtons = primary.locator('.sidebar-sphere-btn');
    await expect(sphereButtons).toHaveCount(2);
    await expect(sphereButtons.filter({ hasText: 'Work' })).toBeVisible();
    await expect(sphereButtons.filter({ hasText: 'Private' })).toBeVisible();
    await expect(primary.locator('.sidebar-sphere-btn.is-active')).toHaveText(/Private/i);

    const pin = page.locator('#sidebar-workspace-pin');
    await expect(pin).toBeVisible();
    await expect(pin.locator('.sidebar-workspace-pin-kicker')).toHaveText(/Workspace/i);

    const capture = page.locator('#sidebar-capture-trigger');
    await expect(capture).toBeVisible();
    await expect(capture).toHaveText(/Capture/i);

    const layout = await page.evaluate(() => {
      const primaryEl = document.getElementById('sidebar-primary');
      const tabs = document.querySelector('#pr-file-list .sidebar-tabs');
      const filesTab = document.querySelectorAll('#pr-file-list .sidebar-tab');
      const filesLabel = Array.from(filesTab).map((el) => (el as HTMLElement).textContent?.trim() || '').filter(Boolean);
      if (!(primaryEl instanceof HTMLElement) || !(tabs instanceof HTMLElement)) {
        return null;
      }
      return {
        primaryTop: primaryEl.getBoundingClientRect().top,
        tabsTop: tabs.getBoundingClientRect().top,
        filesLabel,
      };
    });
    expect(layout).not.toBeNull();
    expect(layout?.primaryTop).toBeLessThanOrEqual((layout?.tabsTop || 0));
    expect(layout?.filesLabel).toEqual(expect.arrayContaining(['Files']));
  });

  test('expandable secondary section keeps project items as filters with backend counts and source pills', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, {
      projectItemsOpen: 3,
      peopleOpen: 4,
      driftReview: 1,
      dedupReview: 2,
      recentMeetings: 2,
    });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });

    const toggle = page.locator('#sidebar-secondary-toggle');
    await expect(toggle).toBeVisible();
    await expect(toggle).toHaveText(/Filters & sources/i);

    const body = page.locator('#sidebar-secondary-body');
    await expect(body).toBeHidden();
    await toggle.click();
    await expect(body).toBeVisible();

    const projectRow = body.locator('.sidebar-secondary-row[data-section-id="project-items"]');
    await expect(projectRow).toBeVisible();
    await expect(projectRow.locator('.sidebar-secondary-row-label')).toHaveText('Project items');
    await expect(projectRow.locator('.sidebar-secondary-row-count')).toHaveText('3');

    await expect(body.locator('.sidebar-secondary-row[data-section-id="people"] .sidebar-secondary-row-count')).toHaveText('4');
    await expect(body.locator('.sidebar-secondary-row[data-section-id="drift"] .sidebar-secondary-row-count')).toHaveText('1');
    await expect(body.locator('.sidebar-secondary-row[data-section-id="dedup"] .sidebar-secondary-row-count')).toHaveText('2');
    const meetingsRow = body.locator('.sidebar-secondary-row[data-section-id="recent-meetings"]');
    await expect(meetingsRow.locator('.sidebar-secondary-row-count')).toHaveText('2');

    const sourceLabels = await body.locator('.sidebar-source-pill').allTextContents();
    expect(sourceLabels).toEqual(['Email', 'Todoist', 'GitHub', 'GitLab', 'Markdown', 'Local']);

    await body.locator('.sidebar-source-pill[data-source-id="github"]').click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const filters = app?.getState?.().itemSidebarFilters || {};
      return String(filters.source || '');
    })).toBe('github');
    await expect(page.locator('.sidebar-source-pill[data-source-id="github"]')).toHaveClass(/is-active/);
  });

  test('clicking a secondary row drills the queue down with a section filter', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, {
      projectItemsOpen: 3,
      peopleOpen: 0,
      driftReview: 0,
      dedupReview: 0,
      recentMeetings: 0,
    });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });
    await page.locator('#sidebar-secondary-toggle').click();

    await page.locator('.sidebar-secondary-row[data-section-id="project-items"]').click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const filters = app?.getState?.().itemSidebarFilters || {};
      return String(filters.section || '');
    })).toBe('project_items');
    await expect(page.locator('.sidebar-secondary-row[data-section-id="project-items"]')).toHaveClass(/is-active/);

    await page.locator('.sidebar-secondary-row[data-section-id="project-items"]').click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const filters = app?.getState?.().itemSidebarFilters || {};
      return String(filters.section || '');
    })).toBe('');
  });

  test('clicking the recent-meetings row drills into review with a recent_meetings filter', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, {
      projectItemsOpen: 0,
      peopleOpen: 0,
      driftReview: 0,
      dedupReview: 0,
      recentMeetings: 4,
    });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });
    await page.locator('#sidebar-secondary-toggle').click();

    const meetingsRow = page.locator('.sidebar-secondary-row[data-section-id="recent-meetings"]');
    await expect(meetingsRow.locator('.sidebar-secondary-row-count')).toHaveText('4');

    await meetingsRow.click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const s = app?.getState?.() || {};
      return {
        section: String(s.itemSidebarFilters?.section || ''),
        view: String(s.itemSidebarView || ''),
      };
    })).toEqual({ section: 'recent_meetings', view: 'review' });
    await expect(meetingsRow).toHaveClass(/is-active/);
    await expect(page.locator('.sidebar-tab.is-active')).toContainText(/Review/i);

    await meetingsRow.click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const filters = app?.getState?.().itemSidebarFilters || {};
      return String(filters.section || '');
    })).toBe('');
  });

  test('does not conflate project items with the active workspace pin', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, { projectItemsOpen: 5, recentMeetings: 0 });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });
    await page.locator('#sidebar-secondary-toggle').click();

    const pinName = await page.locator('#sidebar-workspace-pin .sidebar-workspace-pin-name').innerText();
    const projectRow = page.locator('.sidebar-secondary-row[data-section-id="project-items"]');
    const projectLabel = await projectRow.locator('.sidebar-secondary-row-label').innerText();

    expect(projectLabel).toBe('Project items');
    expect(pinName.trim().length).toBeGreaterThan(0);
    expect(pinName.trim()).not.toBe(projectLabel);

    const pinKicker = await page.locator('#sidebar-workspace-pin .sidebar-workspace-pin-kicker').innerText();
    expect(pinKicker.trim().toLowerCase()).toBe('workspace');
  });

  test('narrow viewport does not overlap or hide labels in the primary header', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 720 });
    await waitReady(page);
    await openInbox(page);

    const layout = await page.evaluate(() => {
      const sphereRow = document.getElementById('sidebar-sphere-row');
      const pinRow = document.getElementById('sidebar-workspace-pin');
      const capture = document.getElementById('sidebar-capture-trigger');
      if (!(sphereRow instanceof HTMLElement) || !(pinRow instanceof HTMLElement) || !(capture instanceof HTMLElement)) {
        return null;
      }
      const sphereRect = sphereRow.getBoundingClientRect();
      const pinRect = pinRow.getBoundingClientRect();
      const captureRect = capture.getBoundingClientRect();
      return {
        sphereBottom: sphereRect.bottom,
        pinTop: pinRect.top,
        pinBottom: pinRect.bottom,
        captureTop: captureRect.top,
        sphereWidth: sphereRect.width,
        captureLabel: capture.textContent?.trim() || '',
      };
    });
    expect(layout).not.toBeNull();
    expect(layout?.sphereBottom).toBeLessThanOrEqual((layout?.pinTop || 0) + 1);
    expect(layout?.pinBottom).toBeLessThanOrEqual((layout?.captureTop || 0) + 1);
    expect(layout?.sphereWidth).toBeGreaterThan(0);
    expect(layout?.captureLabel).toMatch(/Capture/i);
  });
});
