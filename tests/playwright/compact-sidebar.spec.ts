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
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-item-sidebar-ui.js');
    await mod.openItemSidebarView('inbox', { section: '' });
  });
  await expect.poll(async () => page.evaluate(() => {
    const state = (window as any)._slopshellApp?.getState?.() || {};
    return String(state.itemSidebarView || '');
  })).toBe('inbox');
}

async function switchHarnessWorkspace(page: Page, workspaceID: string) {
  await page.evaluate(async (id) => {
    const mod = await import('../../internal/web/static/app-chat-transport.js');
    await mod.switchProject(id);
  }, workspaceID);
  await expect.poll(async () => page.evaluate(() => {
    const state = (window as any)._slopshellApp?.getState?.() || {};
    return String(state.activeWorkspaceId || '');
  })).toBe(workspaceID);
}

async function openNextQueue(page: Page) {
  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-item-sidebar-ui.js');
    await mod.openItemSidebarView('next', { section: '' });
  });
  await expect(page.locator('.sidebar-tab.is-active')).toContainText('Next');
}

async function seedRuntimeWorkspaces(page: Page, activeWorkspaceID = '1') {
  await page.evaluate(async (activeID) => {
    (window as any).__harnessLog = [];
    (window as any).__setProjects([
      { id: '1', name: 'Brain', root_path: '/tmp/brain', workspace_path: '/tmp/brain', sphere: 'private' },
      { id: '2', name: 'Lab', root_path: '/tmp/lab', workspace_path: '/tmp/lab', sphere: 'private' },
    ], activeID);
    (window as any).__setItemSidebarWorkspaces([
      { id: 1, name: 'Brain', dir_path: '/tmp/brain', sphere: 'private', is_active: activeID === '1' },
      { id: 2, name: 'Lab', dir_path: '/tmp/lab', sphere: 'private', is_active: activeID === '2' },
    ]);
    await (window as any)._slopshellApp?.fetchProjects?.();
  }, activeWorkspaceID);
  await switchHarnessWorkspace(page, activeWorkspaceID);
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
    expect(layout?.filesLabel).toEqual(expect.arrayContaining(['Active', 'Inbox2', 'Files']));
  });

  test('inbox is discoverable only when it has uncategorized items or is selected', async ({ page }) => {
    await waitReady(page);
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [{ id: 701, title: 'Queued action', state: 'next', sphere: 'private' }],
        waiting: [],
        deferred: [],
        someday: [],
        review: [],
        done: [],
      });
      (window as any).__itemSidebarSectionCounts = { project_items_open: 1 };
    });
    await page.locator('#edge-left-tap').click();
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });

    await expect(page.locator('.sidebar-tab', { hasText: /^Inbox/ })).toHaveCount(0);

    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [{ id: 702, title: 'New uncategorized mail', state: 'inbox', sphere: 'private', artifact_kind: 'email' }],
        next: [{ id: 701, title: 'Queued action', state: 'next', sphere: 'private' }],
        waiting: [],
        deferred: [],
        someday: [],
        review: [],
        done: [],
      });
    });
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });

    const inboxTab = page.locator('.sidebar-tab', { hasText: /^Inbox/ });
    await expect(inboxTab).toHaveText(/Inbox1/);
    await inboxTab.click();
    await expect(page.locator('#pr-file-list .pr-file-item[data-item-id="702"]')).toContainText('New uncategorized mail');
  });

  test('done and files use phone-sized sidebar targets', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 760 });
    await waitReady(page);
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [{ id: 704, title: 'Loose inbox item', state: 'inbox', sphere: 'private' }],
        next: [],
        waiting: [],
        deferred: [],
        someday: [],
        review: [],
        done: [{ id: 703, title: 'Closed task', state: 'done', sphere: 'private' }],
      });
    });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });

    const doneTab = page.locator('.sidebar-tab', { hasText: /^Done/ });
    const filesTab = page.locator('.sidebar-tab', { hasText: /^Files$/ });
    await expect(doneTab).toBeVisible();
    await expect(filesTab).toBeVisible();

    const boxes = await Promise.all([doneTab.boundingBox(), filesTab.boundingBox()]);
    for (const box of boxes) {
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(48);
      expect(box?.width ?? 0).toBeGreaterThanOrEqual(120);
    }

    await doneTab.click();
    await expect(page.locator('#pr-file-list .pr-file-item[data-item-id="703"]')).toContainText('Closed task');
    await filesTab.click();
    await expect(filesTab).toHaveClass(/is-active/);
  });

  test('activating a linked action reopens its remembered workspace', async ({ page }) => {
    await waitReady(page);
    await seedRuntimeWorkspaces(page, '1');
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [{ id: 801, title: 'Tune lab notebook', state: 'next', sphere: 'private', workspace_id: 2 }],
        waiting: [],
        deferred: [],
        someday: [],
        review: [],
        done: [],
      });
    });

    await openNextQueue(page);
    await page.locator('#pr-file-list .pr-file-item[data-item-id="801"]').click();

    await expect.poll(async () => page.evaluate(() => {
      const state = (window as any)._slopshellApp?.getState?.() || {};
      return String(state.activeWorkspaceId || '');
    })).toBe('2');
    await expect.poll(async () => page.evaluate(() => {
      const log = (window as any).__harnessLog || [];
      return log.some((entry: any) => entry?.action === 'project_activate' && String(entry?.payload?.workspace_id || '') === '2');
    })).toBe(true);
  });

  test('activating an unlinked action remembers the current non-default workspace', async ({ page }) => {
    await waitReady(page);
    await seedRuntimeWorkspaces(page, '2');
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [{ id: 802, title: 'Record lab follow-up', state: 'next', sphere: 'private' }],
        waiting: [],
        deferred: [],
        someday: [],
        review: [],
        done: [],
      });
    });

    await openNextQueue(page);
    await page.locator('#pr-file-list .pr-file-item[data-item-id="802"]').click();

    await expect.poll(async () => page.evaluate(() => {
      const data = (window as any).__itemSidebarData || {};
      return Number(data.next?.find((item: any) => Number(item?.id || 0) === 802)?.workspace_id || 0);
    })).toBe(2);
    await expect.poll(async () => page.evaluate(() => {
      const log = (window as any).__harnessLog || [];
      return log.some((entry: any) => entry?.action === 'item_workspace' && Number(entry?.payload?.workspace_id || 0) === 2);
    })).toBe(true);
  });

  test('capture button opens the composer', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 760 });
    await waitReady(page);
    await openInbox(page);

    await page.locator('#sidebar-capture-trigger').click();

    await expect(page.locator('#floating-input')).toBeVisible();
    await expect(page.locator('#floating-input')).toBeFocused();
  });

  test('plain task fallback shows project context instead of raw source metadata', async ({ page }) => {
    await waitReady(page);

    const markdown = await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-artifacts.js');
      return mod.buildSidebarItemFallbackText({
        id: 42,
        title: 'PR: Build on Linux on ARM',
        kind: 'action',
        state: 'next',
        source: 'todoist',
        source_ref: 'task:6XX5mm2JpV3wGC63',
        project_item_id: 7,
        project_item_title: 'Build portability',
      }, null);
    });

    expect(markdown).toContain('# PR: Build on Linux on ARM');
    expect(markdown).toContain('## Backlinks');
    expect(markdown).toContain('- Project: Build portability');
    expect(markdown).not.toContain('Kind:');
    expect(markdown).not.toContain('Source: task:');
  });

  test('projects are reachable as a first-level tab', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 760 });
    await waitReady(page);
    await seedSectionFixture(page, { projectItemsOpen: 2 });
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [
          { id: 900, title: 'Ship mobile project view', kind: 'project', state: 'next', sphere: 'private' },
          { id: 901, title: 'Open mobile project review', state: 'next', sphere: 'private', project_item_id: 900 },
        ],
        waiting: [],
        deferred: [],
        someday: [],
        done: [],
      });
    });
    await openInbox(page);
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
      await mod.refreshItemSidebarCounts();
    });

    const projectsTab = page.locator('.sidebar-tab', { hasText: 'Active' });
    await expect(projectsTab).toBeVisible();
    await projectsTab.click();
    await expect(projectsTab).toHaveClass(/is-active/);
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const filters = app?.getState?.().itemSidebarFilters || {};
      return String(filters.section || '');
    })).toBe('project_items');
    await expect(page.locator('#pr-file-list .pr-file-item[data-item-id="900"]')).toContainText('Ship mobile project view');
  });

  test('collapsed secondary section keeps only non-empty filters and tucks source pills away', async ({ page }) => {
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
    await expect(toggle).toHaveText(/More/i);

    const body = page.locator('#sidebar-secondary-body');
    await expect(body).toBeHidden();
    await toggle.click();
    await expect(body).toBeVisible();

    const projectRow = body.locator('.sidebar-secondary-row[data-section-id="project-items"]');
    await expect(projectRow).toBeVisible();
    await expect(projectRow.locator('.sidebar-secondary-row-label')).toHaveText('Active projects');
    await expect(projectRow.locator('.sidebar-secondary-row-count')).toHaveText('3');

    await expect(body.locator('.sidebar-secondary-row[data-section-id="people"] .sidebar-secondary-row-count')).toHaveText('4');
    await expect(body.locator('.sidebar-secondary-row[data-section-id="drift"] .sidebar-secondary-row-count')).toHaveText('1');
    await expect(body.locator('.sidebar-secondary-row[data-section-id="dedup"] .sidebar-secondary-row-count')).toHaveText('2');
    const meetingsRow = body.locator('.sidebar-secondary-row[data-section-id="recent-meetings"]');
    await expect(meetingsRow.locator('.sidebar-secondary-row-count')).toHaveText('2');

    const sourceDetails = body.locator('#sidebar-secondary-sources');
    await expect(sourceDetails).toBeVisible();
    await expect(sourceDetails.locator('.sidebar-secondary-sources-label')).toHaveText('Source');
    await sourceDetails.locator('.sidebar-secondary-sources-label').click();
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

  test('project section stays title-only and opens a child-action queue', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, {
      projectItemsOpen: 2,
      peopleOpen: 0,
      driftReview: 0,
      dedupReview: 0,
      recentMeetings: 0,
    });
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [
          { id: 900, title: 'Ship compact outcome', kind: 'project', state: 'next', sphere: 'private' },
          { id: 901, title: 'Work-only outcome', kind: 'project', state: 'next', sphere: 'work' },
          {
            id: 101,
            title: 'Write rollout note',
            state: 'next',
            sphere: 'private',
            project_item_id: 900,
            project_item_title: 'Ship compact outcome',
            follow_up_at: '2026-05-02T08:00:00Z',
            due_at: '2026-05-06T17:00:00Z',
          },
          { id: 102, title: 'Unlinked next action', state: 'next', sphere: 'private', project_item_id: 999 },
        ],
        waiting: [
          { id: 103, title: 'Waiting on reviewer', state: 'waiting', sphere: 'private', project_item_id: 900 },
        ],
        deferred: [
          { id: 104, title: 'Check release date', state: 'deferred', sphere: 'private', project_item_id: 900 },
        ],
        someday: [
          { id: 105, title: 'Consider polish pass', state: 'someday', sphere: 'private', project_item_id: 900 },
        ],
        done: [
          { id: 106, title: 'Closed design note', state: 'done', sphere: 'private', project_item_id: 900 },
        ],
      });
    });
    await openInbox(page);
    await page.locator('#sidebar-secondary-toggle').click();
    await page.locator('.sidebar-secondary-row[data-section-id="project-items"]').click();

    const projectRow = page.locator('#pr-file-list .pr-file-item[data-item-id="900"]');
    await expect(projectRow).toBeVisible();
    await expect(projectRow).toHaveText('Ship compact outcome');
    await expect(projectRow.locator('.sidebar-row-secondary')).toHaveCount(0);
    await expect(projectRow.locator('.pr-file-status')).toHaveCount(0);
    await expect(page.locator('#pr-file-list')).not.toContainText('Work-only outcome');

    await projectRow.click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const s = app?.getState?.() || {};
      return {
        projectItemID: Number(s.itemSidebarFilters?.project_item_id || 0),
        section: String(s.itemSidebarFilters?.section || ''),
        view: String(s.itemSidebarView || ''),
      };
    })).toEqual({ projectItemID: 900, section: '', view: 'next' });
    const childRow = page.locator('#pr-file-list .pr-file-item[data-item-id="101"]');
    await expect(childRow).toHaveText('Write rollout note');
    await expect(childRow.locator('.sidebar-row-secondary')).toHaveCount(0);
    await expect(childRow).toHaveAttribute('data-deadline', 'soon');
    await expect(page.locator('#pr-file-list')).not.toContainText('Unlinked next action');
  });

  test('project section has a clear empty state', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, { projectItemsOpen: 0 });
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        next: [],
        waiting: [],
        deferred: [],
        someday: [],
        done: [],
      });
    });
    await openInbox(page);
    await page.locator('.sidebar-tab', { hasText: 'Active' }).click();

    await expect(page.locator('#pr-file-list .pr-file-item')).toContainText('No active projects.');
  });

  test('people section lists open-loop counts and drills into per-person queues', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSectionFixture(page, {
      projectItemsOpen: 1,
      peopleOpen: 2,
      driftReview: 0,
      dedupReview: 0,
      recentMeetings: 0,
    });
    await page.evaluate(() => {
      (window as any).__itemSidebarActors = [
        { id: 1, name: 'Ada Example', kind: 'human', meta_json: '{"person_path":"brain/people/Ada Example.md"}' },
        { id: 2, name: 'Missing Person', kind: 'human' },
      ];
      (window as any).__setItemSidebarData({
        inbox: [
          { id: 101, title: 'Send Ada answer', state: 'inbox', sphere: 'private', actor_id: 1, actor_name: 'Ada Example', project_item_id: 900 },
          { id: 201, title: 'Clarify missing note', state: 'inbox', sphere: 'private', actor_id: 2, actor_name: 'Missing Person' },
        ],
        waiting: [
          { id: 102, title: 'Waiting for Ada draft', state: 'waiting', sphere: 'private', actor_id: 1, actor_name: 'Ada Example', project_item_id: 900 },
        ],
        deferred: [],
        someday: [],
        done: [
          { id: 103, title: 'Closed Ada thread', state: 'done', sphere: 'private', actor_id: 1, actor_name: 'Ada Example', project_item_id: 900 },
        ],
        next: [
          { id: 900, title: 'Ada collaboration outcome', kind: 'project', state: 'next', sphere: 'private' },
        ],
      });
    });
    await openInbox(page);
    await page.locator('#sidebar-secondary-toggle').click();
    await page.locator('.sidebar-secondary-row[data-section-id="people"]').click();

    const adaRow = page.locator('#pr-file-list .pr-file-item[data-item-id="1"]');
    await expect(adaRow).toBeVisible();
    await expect(adaRow).toHaveText('Ada Example');
    await expect(adaRow.locator('.sidebar-row-secondary')).toHaveCount(0);

    await adaRow.click();
    await expect.poll(async () => page.evaluate(() => {
      const app = (window as any)._slopshellApp;
      const s = app?.getState?.() || {};
      return {
        actorID: Number(s.itemSidebarFilters?.actor_id || 0),
        section: String(s.itemSidebarFilters?.section || ''),
      };
    })).toEqual({ actorID: 1, section: 'people' });
    await expect(page.locator('#pr-file-list')).toContainText('Waiting on them (1)');
    await expect(page.locator('#pr-file-list')).toContainText('I owe them (1)');
    await expect(page.locator('#pr-file-list')).toContainText('Recently closed (1)');
    await expect(page.locator('#pr-file-list')).toContainText('Projects (1)');
    await expect(page.locator('#pr-file-list')).toContainText('Ada collaboration outcome');
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

  test('does not conflate projects with the active workspace pin', async ({ page }) => {
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

    expect(projectLabel).toBe('Active projects');
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
        minTabHeight: Math.min(...Array.from(document.querySelectorAll('.sidebar-tab, .pr-file-item')).map((el) => (el as HTMLElement).getBoundingClientRect().height)),
      };
    });
    expect(layout).not.toBeNull();
    expect(layout?.sphereBottom).toBeLessThanOrEqual((layout?.pinTop || 0) + 1);
    expect(layout?.pinBottom).toBeLessThanOrEqual((layout?.captureTop || 0) + 1);
    expect(layout?.sphereWidth).toBeGreaterThan(0);
    expect(layout?.captureLabel).toMatch(/Capture/i);
    expect(layout?.minTabHeight).toBeGreaterThanOrEqual(47);
  });

  test('mobile right panel keeps the circle clear of the message input', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 760 });
    await waitReady(page);

    await page.locator('#edge-right-tap').click();
    await expect(page.locator('#edge-right')).toHaveClass(/edge-pinned/);

    const layout = await page.evaluate(() => {
      const dot = document.getElementById('slopshell-circle-dot');
      const input = document.getElementById('chat-pane-input');
      if (!(dot instanceof HTMLElement) || !(input instanceof HTMLElement)) return null;
      const dotRect = dot.getBoundingClientRect();
      const inputRect = input.getBoundingClientRect();
      return {
        dotBottom: dotRect.bottom,
        inputTop: inputRect.top,
        inputHeight: inputRect.height,
      };
    });
    expect(layout).not.toBeNull();
    expect(layout?.dotBottom).toBeLessThanOrEqual((layout?.inputTop || 0) - 4);
    expect(layout?.inputHeight).toBeGreaterThanOrEqual(47);
  });
});
