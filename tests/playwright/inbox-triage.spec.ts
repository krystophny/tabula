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

async function openInbox(page: Page) {
  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  const inboxTab = page.locator('.sidebar-tab', { hasText: 'Inbox' });
  await expect(inboxTab).toBeVisible();
  if (!(await inboxTab.getAttribute('class'))?.includes('is-active')) {
    await inboxTab.click();
  }
  await expect(page.locator('.sidebar-tab.is-active')).toContainText('Inbox');
}

async function refreshItemSidebarCounts(page: Page) {
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-item-sidebar-utils.js');
    await mod.refreshItemSidebarCounts();
  });
}

async function clickSphereButton(page: Page, sphere: 'work' | 'private') {
  await page.evaluate(async (nextSphere) => {
    const mod = await import('../../internal/web/static/app-workspace-runtime.js');
    await mod.setActiveSphere(nextSphere);
  }, sphere);
}

async function refreshProjects(page: Page) {
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-workspace-runtime.js');
    await mod.fetchProjects();
  });
}

async function projectButtonTexts(page: Page) {
  return page.evaluate(() => {
    return Array.from(document.querySelectorAll('#edge-top-projects .edge-project-btn'))
      .map((button) => String(button.textContent || '').trim())
      .filter(Boolean);
  });
}

async function seedSphereScenario(page: Page) {
  await page.evaluate(() => {
    (window as any).__setProjects([
      {
        id: 'private-notes',
        name: 'Private Notes',
        kind: 'managed',
        sphere: 'private',
        workspace_path: '/tmp/private-notes',
        root_path: '/tmp/private-notes',
        chat_session_id: 'chat-private',
        canvas_session_id: 'local',
      },
      {
        id: 'work-tracker',
        name: 'Work Tracker',
        kind: 'managed',
        sphere: 'work',
        workspace_path: '/tmp/work-tracker',
        root_path: '/tmp/work-tracker',
        chat_session_id: 'chat-work',
        canvas_session_id: 'local',
      },
    ], 'private-notes');
    (window as any).__setRuntimeState({ active_sphere: 'private' });
    (window as any).__setItemSidebarWorkspaces([
      { id: 1, name: 'Private Desk', dir_path: '/tmp/private', sphere: 'private', is_active: true },
      { id: 2, name: 'Work Desk', dir_path: '/tmp/work', sphere: 'work', is_active: false },
    ]);
    (window as any).__setItemSidebarData({
      inbox: [
        {
          id: 101,
          title: 'Private inbox item',
          state: 'inbox',
          sphere: 'private',
          artifact_id: 501,
          artifact_title: 'Parser cleanup plan',
          artifact_kind: 'idea_note',
          actor_name: 'Alice',
          created_at: '2026-03-08 09:40:00',
          updated_at: '2026-03-08 09:58:00',
        },
        {
          id: 102,
          title: 'Work inbox item',
          state: 'inbox',
          sphere: 'work',
          artifact_id: 502,
          artifact_title: 'Re: triage follow-up',
          artifact_kind: 'email',
          actor_name: 'Bob',
          created_at: '2026-03-08 09:10:00',
          updated_at: '2026-03-08 09:12:00',
        },
      ],
      waiting: [],
      someday: [],
      done: [],
    });
    localStorage.removeItem('slopshell.activeSphere');
    document.getElementById('edge-top')?.classList.add('edge-pinned');
  });
  await refreshProjects(page);
  await refreshItemSidebarCounts(page);
}

const parserInboxItem = {
  id: 101,
  title: 'Review parser cleanup',
  state: 'inbox',
  sphere: 'private',
  artifact_id: 501,
  source: 'github',
  source_ref: 'owner/repo#177',
  artifact_title: 'Parser cleanup plan',
  artifact_kind: 'idea_note',
  actor_name: 'Alice',
  created_at: '2026-03-08 09:40:00',
  updated_at: '2026-03-08 09:58:00',
};

const privateEmailInboxItem = {
  id: 102,
  title: 'Answer triage email',
  state: 'inbox',
  sphere: 'private',
  artifact_id: 502,
  source: 'exchange',
  source_ref: 'msg-102',
  artifact_title: 'Re: triage follow-up',
  artifact_kind: 'email',
  actor_name: 'Bob',
  created_at: '2026-03-08 09:10:00',
  updated_at: '2026-03-08 09:12:00',
};

const reopenedEmailItem = {
  ...privateEmailInboxItem,
  state: 'done',
};

const mixedSourceArtifacts = {
  701: { id: 701, kind: 'markdown', title: 'Loose markdown capture', meta_json: JSON.stringify({ text: 'Needs routing.' }) },
  702: { id: 702, kind: 'external_task', title: 'Todoist inbox task', meta_json: JSON.stringify({ text: 'Clarify todoist.' }) },
  703: { id: 703, kind: 'email', title: 'Read inbox mail', meta_json: JSON.stringify({ is_read: true, body: 'Read but still open.' }) },
  704: { id: 704, kind: 'idea_note', title: 'Local voice capture', meta_json: JSON.stringify({ transcript: 'Local capture.' }) },
  705: { id: 705, kind: 'external_note', title: 'Imported candidate', meta_json: JSON.stringify({ text: 'Imported candidate.' }) },
};

const mixedSourceInboxItems = [
  {
    id: 701,
    title: 'Loose markdown capture',
    state: 'inbox',
    sphere: 'private',
    workspace_id: 1,
    artifact_id: 701,
    source: 'markdown',
    source_ref: 'brain/inbox/loose.md',
    artifact_title: 'Loose markdown capture',
    artifact_kind: 'markdown',
    updated_at: '2026-03-08 10:10:00',
  },
  {
    id: 702,
    title: 'Todoist inbox task',
    state: 'inbox',
    sphere: 'private',
    artifact_id: 702,
    source: 'todoist',
    source_ref: 'https://todoist.com/showTask?id=702',
    artifact_title: 'Todoist inbox task',
    artifact_kind: 'external_task',
    updated_at: '2026-03-08 10:09:00',
  },
  {
    id: 703,
    title: 'Read inbox mail',
    state: 'inbox',
    sphere: 'private',
    artifact_id: 703,
    source: 'exchange',
    source_ref: 'message:read-inbox',
    artifact_title: 'Read inbox mail',
    artifact_kind: 'email',
    updated_at: '2026-03-08 10:08:00',
  },
  {
    id: 704,
    title: 'Local voice capture',
    state: 'inbox',
    sphere: 'private',
    artifact_id: 704,
    source: 'local',
    source_ref: 'capture:voice-1',
    artifact_title: 'Local voice capture',
    artifact_kind: 'idea_note',
    updated_at: '2026-03-08 10:07:00',
  },
  {
    id: 705,
    title: 'Imported candidate',
    state: 'inbox',
    sphere: 'private',
    artifact_id: 705,
    source: 'evernote',
    source_ref: 'note:imported-1',
    artifact_title: 'Imported candidate',
    artifact_kind: 'external_note',
    updated_at: '2026-03-08 10:06:00',
  },
];

const mixedSourceProjectItems = [{
  id: 900,
  title: 'Unified Inbox Outcome',
  kind: 'project',
  state: 'next',
  sphere: 'private',
  artifact_id: 0,
  source: '',
  source_ref: '',
  updated_at: '2026-03-08 10:00:00',
}];

const mixedSourceTitles = mixedSourceInboxItems.map((item) => item.title);

async function setInboxItems(page: Page, items: Array<Record<string, unknown>>) {
  await page.evaluate((nextItems) => {
    (window as any).__setItemSidebarData({ inbox: nextItems });
  }, items);
  await page.locator('.sidebar-tab', { hasText: 'Inbox' }).evaluate((el: HTMLElement) => {
    el.click();
  });
  if (items.length > 0) {
    await expect(page.locator('#pr-file-list')).toContainText(String(items[0].title || ''));
  } else {
    await expect(page.locator('#pr-file-list')).toContainText('No inbox items.');
  }
}

async function queueSidebarResponseDelay(
  page: Page,
  view: 'inbox' | 'waiting' | 'someday' | 'done' | 'counts',
  delayMs: number,
) {
  await page.evaluate(({ nextView, nextDelayMs }) => {
    (window as any).__queueItemSidebarResponseDelay(nextView, nextDelayMs);
  }, { nextView: view, nextDelayMs: delayMs });
}

async function swipeCanvas(page: Page, dx: number, dy = 0) {
  await page.locator('#canvas-viewport').evaluate((el, payload) => {
    const target = el as HTMLElement;
    const rect = target.getBoundingClientRect();
    const startX = rect.left + rect.width / 2;
    const startY = rect.top + rect.height / 2;
    const endX = startX + Number(payload.dx || 0);
    const endY = startY + Number(payload.dy || 0);
    const makeTouch = (clientX: number, clientY: number) => new Touch({
      identifier: 1,
      target,
      clientX,
      clientY,
      pageX: clientX,
      pageY: clientY,
      screenX: clientX,
      screenY: clientY,
    });
    const startTouch = makeTouch(startX, startY);
    const endTouch = makeTouch(endX, endY);
    target.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: [startTouch],
      changedTouches: [startTouch],
      targetTouches: [startTouch],
    }));
    target.dispatchEvent(new TouchEvent('touchmove', {
      bubbles: true,
      cancelable: true,
      touches: [endTouch],
      changedTouches: [endTouch],
      targetTouches: [endTouch],
    }));
    target.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      changedTouches: [endTouch],
      targetTouches: [],
    }));
  }, { dx, dy });
}

async function touchPhase(page: Page, selector: string, phase: 'start' | 'move' | 'end', dx = 0, dy = 0) {
  await page.locator(selector).evaluate((el, payload) => {
    const target = el as HTMLElement;
    const key = String(payload.selector || '');
    const touchState = ((window as any).__playwrightTouchState ||= {});
    const readOrigin = () => {
      const existing = touchState[key];
      if (existing && Number.isFinite(existing.startX) && Number.isFinite(existing.startY)) {
        return existing;
      }
      const rect = target.getBoundingClientRect();
      const origin = {
        startX: rect.left + 24,
        startY: rect.top + rect.height / 2,
      };
      touchState[key] = origin;
      return origin;
    };
    const makeTouch = (clientX: number, clientY: number) => new Touch({
      identifier: 1,
      target,
      clientX,
      clientY,
      pageX: clientX,
      pageY: clientY,
      screenX: clientX,
      screenY: clientY,
    });
    if (payload.phase === 'start') {
      const rect = target.getBoundingClientRect();
      const origin = {
        startX: rect.left + 24,
        startY: rect.top + rect.height / 2,
      };
      touchState[key] = origin;
      const startTouch = makeTouch(origin.startX, origin.startY);
      target.dispatchEvent(new TouchEvent('touchstart', {
        bubbles: true,
        cancelable: true,
        touches: [startTouch],
        changedTouches: [startTouch],
        targetTouches: [startTouch],
      }));
      return;
    }
    const origin = readOrigin();
    const currentX = origin.startX + Number(payload.dx || 0);
    const currentY = origin.startY + Number(payload.dy || 0);
    const moveTouch = makeTouch(currentX, currentY);
    const startTouch = new Touch({
      identifier: 1,
      target,
      clientX: origin.startX,
      clientY: origin.startY,
      pageX: origin.startX,
      pageY: origin.startY,
      screenX: origin.startX,
      screenY: origin.startY,
    });
    if (payload.phase === 'move') {
      target.dispatchEvent(new TouchEvent('touchmove', {
        bubbles: true,
        cancelable: true,
        touches: [moveTouch],
        changedTouches: [moveTouch],
        targetTouches: [moveTouch],
      }));
      return;
    }
    target.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      changedTouches: [moveTouch],
      targetTouches: [],
    }));
    delete touchState[key];
  }, { selector, phase, dx, dy });
}

async function seedMixedSourceInbox(page: Page) {
  await page.evaluate(({ artifacts, inboxItems, projectItems }) => {
    (window as any).__openedSources = [];
    (window as any).open = (url: string) => {
      (window as any).__openedSources.push(String(url || ''));
      return null;
    };
    (window as any).__setItemSidebarArtifacts(artifacts);
    (window as any).__setItemSidebarData({
      inbox: inboxItems,
      next: projectItems,
      waiting: [],
      someday: [],
      done: [],
    });
  }, {
    artifacts: mixedSourceArtifacts,
    inboxItems: mixedSourceInboxItems,
    projectItems: mixedSourceProjectItems,
  });
}

async function reloadInboxSidebar(page: Page) {
  await openInbox(page);
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-item-sidebar-ui.js');
    await mod.loadItemSidebarView('inbox');
  });
}

async function expectMixedSourceRows(page: Page) {
  for (const title of mixedSourceTitles) {
    await expect(page.locator('#pr-file-list')).toContainText(title);
  }
}

async function openItemActionMenu(page: Page, itemID: number) {
  const row = page.locator(`#pr-file-list .pr-file-item[data-item-id="${itemID}"]`);
  await row.click({ button: 'right' });
  return row;
}

async function expectHarnessLogEntry(page: Page, predicate: (entry: any) => boolean) {
  await expect.poll(async () => {
    const log = await page.evaluate(() => (window as any).__harnessLog || []);
    return log.some(predicate);
  }).toBe(true);
}

async function verifyTodoistSourceAction(page: Page) {
  await openItemActionMenu(page, 702);
  const sourceAuthority = page.locator('#item-sidebar-menu .item-sidebar-menu-item[data-action="source_authority"]');
  await expect(sourceAuthority).toHaveText('Source: todoist');
  await expect(sourceAuthority).toBeDisabled();
  await page.locator('#item-sidebar-menu .item-sidebar-menu-item[data-action="open_source"]').click();
  await expect.poll(async () => {
    return page.evaluate(() => (window as any).__openedSources || []);
  }).toEqual(['https://todoist.com/showTask?id=702']);
}

async function clarifyTodoistCapture(page: Page) {
  await openItemActionMenu(page, 702);
  const clarifyAction = page.locator('#item-sidebar-menu .item-sidebar-menu-item[data-action="next"]');
  await expect(clarifyAction).toHaveText('Clarify...');
  await clarifyAction.click();
  await expectHarnessLogEntry(page, (entry) => {
    return entry?.action === 'item_triage' && entry?.payload?.action === 'next';
  });
}

async function linkMarkdownCaptureToProject(page: Page) {
  await openItemActionMenu(page, 701);
  await page.locator('#item-sidebar-menu .item-sidebar-menu-item[data-action="project_item"]').click();
  await expect(page.locator('#item-sidebar-menu')).toContainText('Unified Inbox Outcome');
  await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Unified Inbox Outcome' }).click();
  await expectHarnessLogEntry(page, (entry) => {
    return entry?.action === 'item_project_item_link' && entry?.payload?.project_item_id === 900;
  });
}

async function materializeMarkdownOnlyOnDemand(page: Page) {
  const materializeCalls = await page.evaluate(() => {
    const log = (window as any).__harnessLog || [];
    return log.filter((entry: any) => entry?.action === 'artifact_materialize').length;
  });
  expect(materializeCalls).toBe(0);
  await openItemActionMenu(page, 701);
  await page.locator('#item-sidebar-menu .item-sidebar-menu-item[data-action="materialize_artifact"]').click();
  await expect.poll(async () => {
    const log = await page.evaluate(() => (window as any).__harnessLog || []);
    return log.find((entry: any) => entry?.action === 'artifact_materialize')?.payload?.relative_path || '';
  }).toBe('.slopshell/artifacts/materialized/701.md');
}

test.describe('inbox triage interactions', () => {
  test('sphere toggle filters projects and inbox items', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSphereScenario(page);

    await expect.poll(() => projectButtonTexts(page)).toEqual(['Private Notes']);
    await expect(page.locator('#edge-left-tap')).toHaveAttribute('data-inbox-count', '1');

    await clickSphereButton(page, 'work');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.action === 'runtime_preferences' && entry?.payload?.active_sphere === 'work');
    }).toBe(true);

    await expect.poll(() => projectButtonTexts(page)).toEqual(['Work Tracker']);
    await expect(page.locator('#edge-left-tap')).toHaveAttribute('data-inbox-count', '1');
    await openInbox(page);
    await expect(page.locator('.sidebar-tab', { hasText: 'Inbox' }).first()).toContainText('1');
    await expect(page.locator('#pr-file-list')).toContainText('Work inbox item');
    await expect(page.locator('#pr-file-list')).not.toContainText('Private inbox item');
    await expect.poll(async () => {
      return page.evaluate(() => (window as any).__getRuntimeState?.().active_sphere || '');
    }).toBe('work');
  });

  test('moving a standalone item to the other sphere hides it until the sphere changes', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [{
          id: 101,
          title: 'Private inbox item',
          state: 'inbox',
          sphere: 'private',
          artifact_id: 501,
          artifact_title: 'Parser cleanup plan',
          artifact_kind: 'idea_note',
          actor_name: 'Alice',
          created_at: '2026-03-08 09:40:00',
          updated_at: '2026-03-08 09:58:00',
        }],
        waiting: [],
        someday: [],
        done: [],
      });
      (window as any).__setRuntimeState({ active_sphere: 'private' });
      document.getElementById('edge-top')?.classList.add('edge-pinned');
      localStorage.removeItem('slopshell.activeSphere');
    });

    await openInbox(page);
    const row = page.locator('#pr-file-list .pr-file-item[data-item-id="101"]');
    await row.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toContainText('Move to Work');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Move to Work' }).click();

    await expect(page.locator('#pr-file-list')).not.toContainText('Private inbox item');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.action === 'item_update' && entry?.payload?.sphere === 'work');
    }).toBe(true);

    await clickSphereButton(page, 'work');
    await expect(page.locator('#pr-file-list')).toContainText('Private inbox item');
  });

  test('switching directly to a project in another sphere syncs the active sphere', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedSphereScenario(page);

    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-chat-transport.js');
      await mod.switchProject('work-tracker');
    });

    await expect.poll(async () => {
      return page.evaluate(() => (window as any).__getRuntimeState?.().active_sphere || '');
    }).toBe('work');
    await expect.poll(async () => {
      return page.evaluate(() => (window as any)._slopshellApp?.getState?.().activeSphere || '');
    }).toBe('work');
  });

  test('tap opens the linked artifact on canvas', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);

    await page.locator('#pr-file-list .pr-file-item').first().click();

    await expect(page.locator('#canvas-text')).toContainText('Parser cleanup plan');
    await expect(page.locator('#canvas-text')).toContainText('Break parser cleanup into a small refactor');
  });

  test('bare items stay in the sidebar without synthetic canvas output', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);
    await setInboxItems(page, [{
      id: 201,
      title: 'Bare sidebar task',
      state: 'inbox',
      sphere: 'private',
      artifact_id: null,
      artifact_title: '',
      artifact_kind: '',
      actor_name: 'Alice',
      created_at: '2026-03-08 10:00:00',
      updated_at: '2026-03-08 10:05:00',
    }]);

    await page.locator('#pr-file-list .pr-file-item[data-item-id="201"]').click();

    await expect(page.locator('#pr-file-list .pr-file-item.is-active[data-item-id="201"]')).toHaveCount(1);
    await expect(page.locator('#canvas-viewport .canvas-pane.is-active')).toHaveCount(0);
    await expect.poll(async () => {
      return page.evaluate(() => (window as any)._slopshellApp?.getState?.().itemSidebarActiveItemID || 0);
    }).toBe(201);
  });

  test('idea items open as structured idea notes on canvas', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);

    await page.locator('#pr-file-list .pr-file-item[data-item-id="101"]').click();

    await expect(page.locator('#canvas-text')).toContainText('Parser cleanup plan');
    await expect(page.locator('#canvas-text')).toContainText('Notes');
    await expect(page.locator('#canvas-text')).toContainText('Label');
    await expect(page.locator('#canvas-text')).toContainText('Workspace: Default');
  });

  test('idea notes render promotion review details on canvas', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await page.evaluate(() => {
      (window as any).__setItemSidebarArtifacts({
        501: {
          id: 501,
          kind: 'idea_note',
          title: 'Parser cleanup plan',
          meta_json: JSON.stringify({
            title: 'Parser cleanup plan',
            transcript: 'Break parser cleanup into a small refactor, a test pass, and one cleanup issue.',
            capture_mode: 'voice',
            captured_at: '2026-03-08T09:40:00Z',
            workspace: 'Default',
            notes: [
              'Draft the rollout checklist',
              'Add regression coverage',
            ],
            promotion_preview: {
              target: 'items',
              candidates: [
                { index: 1, title: 'Draft the rollout checklist', details: 'Draft the rollout checklist' },
                { index: 2, title: 'Add regression coverage', details: 'Add regression coverage' },
              ],
            },
          }),
        },
      });
    });
    await openInbox(page);

    await page.locator('#pr-file-list .pr-file-item[data-item-id="101"]').click();

    await expect(page.locator('#canvas-text')).toContainText('Promotion Review');
    await expect(page.locator('#canvas-text')).toContainText('Draft the rollout checklist');
    await expect(page.locator('#canvas-text')).toContainText('Add regression coverage');
    await expect(page.locator('#canvas-text .canvas-page-indicator')).toContainText('Page 1 / 2');

    await swipeCanvas(page, -160, 0);

    await expect(page.locator('#canvas-text .canvas-page-indicator')).toContainText('Page 2 / 2');
    await expect(page.locator('#canvas-text')).toContainText('create these idea items');
    await expect(page.locator('#canvas-text')).toContainText('Pending: item proposals');
  });

  test('mobile canvas swipe flips between inbox items', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);

    await page.locator('#pr-file-list .pr-file-item[data-item-id="101"]').click();
    await expect(page.locator('#canvas-text')).toContainText('Break parser cleanup into a small refactor');
    await expect(page.locator('#pr-file-list .pr-file-item.is-active[data-item-id="101"]')).toHaveCount(1);

    await swipeCanvas(page, -160, 0);
    await expect(page.locator('#canvas-text')).toContainText('Need a response before tomorrow morning');
    await expect(page.locator('#pr-file-list .pr-file-item.is-active[data-item-id="102"]')).toHaveCount(1);

    await swipeCanvas(page, 160, 0);
    await expect(page.locator('#canvas-text')).toContainText('Break parser cleanup into a small refactor');
    await expect(page.locator('#pr-file-list .pr-file-item.is-active[data-item-id="101"]')).toHaveCount(1);
  });

  test('touch gestures expose feedback and commit complete, drop, defer, and delegate', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);
    await setInboxItems(page, [parserInboxItem]);

    const row = '#pr-file-list .pr-file-item[data-item-id="101"]';

    await touchPhase(page, row, 'start');
    await touchPhase(page, row, 'move', 90, 0);
    await expect(page.locator(row)).toHaveAttribute('data-triage-action', 'complete');
    await expect(page.locator(row)).toHaveAttribute('data-triage-label', 'Done');
    await touchPhase(page, row, 'end', 90, 0);
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await setInboxItems(page, [parserInboxItem]);
    await touchPhase(page, row, 'start');
    await touchPhase(page, row, 'move', 180, 0);
    await expect(page.locator(row)).toHaveAttribute('data-triage-action', 'drop');
    await expect(page.locator(row)).toHaveAttribute('data-triage-label', 'Drop');
    await touchPhase(page, row, 'end', 180, 0);
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await setInboxItems(page, [parserInboxItem]);
    await touchPhase(page, row, 'start');
    await touchPhase(page, row, 'move', -100, 0);
    await expect(page.locator(row)).toHaveAttribute('data-triage-action', 'defer');
    await expect(page.locator(row)).toHaveAttribute('data-triage-label', 'Defer');
    await touchPhase(page, row, 'end', -100, 0);
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await setInboxItems(page, [parserInboxItem]);
    await touchPhase(page, row, 'start');
    await touchPhase(page, row, 'move', -185, 0);
    await expect(page.locator(row)).toHaveAttribute('data-triage-action', 'delegate');
    await expect(page.locator(row)).toHaveAttribute('data-triage-label', 'Delegate');
    await touchPhase(page, row, 'end', -185, 0);
    await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
    await expect(page.locator('#item-sidebar-menu')).toBeVisible();
    await expect(page.locator('#item-sidebar-menu')).toContainText('Alice');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Codex' }).click();
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    const log = await page.evaluate(() => (window as any).__harnessLog || []);
    const gestureCalls = log.filter((entry: any) => entry?.action === 'item_gesture');
    expect(gestureCalls.map((entry: any) => entry?.payload?.action)).toEqual(['complete', 'drop', 'defer', 'delegate']);
  });

  test('desktop context menu workspace picker reassigns the active item', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await page.evaluate(() => {
      (window as any).__setItemSidebarWorkspaces([
        { id: 1, name: 'Alpha', dir_path: '/tmp/alpha', sphere: 'private', is_active: true },
        { id: 2, name: 'Beta', dir_path: '/tmp/beta', sphere: 'private', is_active: false },
      ]);
    });
    await openInbox(page);
    await setInboxItems(page, [{
      ...parserInboxItem,
      workspace_id: 1,
    }]);

    const row = page.locator('#pr-file-list .pr-file-item[data-item-id="101"]');

    await row.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toContainText('Workspace...');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Workspace...' }).click();
    await expect(page.locator('#item-sidebar-menu')).toContainText('Beta');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Beta' }).click();
    await expect.poll(async () => {
      return page.evaluate(() => {
        const log = (window as any).__harnessLog || [];
        return log.some((entry: any) => entry?.action === 'item_workspace');
      });
    }).toBe(true);

    const itemState = await page.evaluate(() => {
      const data = (window as any).__itemSidebarData || {};
      return Array.isArray(data.inbox) ? data.inbox[0] : null;
    });
    expect(itemState?.workspace_id).toBe(2);
  });

  test('mixed-source inbox exposes guarded source actions, project links, and explicit materialization', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await seedMixedSourceInbox(page);
    await reloadInboxSidebar(page);

    await expectMixedSourceRows(page);
    await verifyTodoistSourceAction(page);
    await clarifyTodoistCapture(page);
    await linkMarkdownCaptureToProject(page);
    await materializeMarkdownOnlyOnDemand(page);
  });

  test('stale inbox reload does not resurrect a triaged item', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);

    const row = '#pr-file-list .pr-file-item[data-item-id="101"]';
    await queueSidebarResponseDelay(page, 'inbox', 250);
    await page.locator('.sidebar-tab', { hasText: 'Inbox' }).click();
    await page.waitForTimeout(50);

    await touchPhase(page, row, 'start');
    await touchPhase(page, row, 'move', -100, 0);
    await expect(page.locator(row)).toHaveAttribute('data-triage-action', 'defer');
    await touchPhase(page, row, 'end', -100, 0);
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await page.waitForTimeout(300);
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');
  });

  test('desktop context menu and keyboard shortcuts cover archive, delete, later, and delegate', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await openInbox(page);
    await setInboxItems(page, [parserInboxItem, privateEmailInboxItem]);

    const emailRow = page.locator('#pr-file-list .pr-file-item[data-item-id="102"]');
    await emailRow.click();
    await emailRow.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toBeVisible();
    await expect(page.locator('#item-sidebar-menu .item-sidebar-menu-item').first()).toContainText('Archive');
    await expect(page.locator('#item-sidebar-menu')).toContainText('Delete');
    await page.mouse.click(10, 10);

    await page.keyboard.press('KeyD');
    await expect(page.locator('#pr-file-list')).not.toContainText('Answer triage email');

    await setInboxItems(page, [parserInboxItem]);
    await page.keyboard.press('KeyL');
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await setInboxItems(page, [parserInboxItem]);
    await page.locator('#pr-file-list .pr-file-item[data-item-id="101"]').click();
    await page.keyboard.press('KeyG');
    await expect(page.locator('#item-sidebar-menu')).toBeVisible();
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Bob' }).click();
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');

    await setInboxItems(page, [parserInboxItem]);
    await page.keyboard.press('Backspace');
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');
  });

  test('non-inbox items support context menus and can move back to inbox', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);
    await openInbox(page);
    await page.evaluate((doneItem) => {
      (window as any).__setItemSidebarData({
        inbox: [],
        waiting: [{
          ...doneItem,
          id: 101,
          title: 'Follow up with Bob',
          state: 'waiting',
          artifact_id: 501,
          artifact_kind: 'idea_note',
          artifact_title: 'Parser cleanup plan',
          source: 'github',
          source_ref: 'owner/repo#177',
        }],
        someday: [],
        done: [doneItem],
      });
    }, reopenedEmailItem);

    await page.locator('.sidebar-tab', { hasText: 'Waiting' }).click();
    const waitingRow = page.locator('#pr-file-list .pr-file-item[data-item-id="101"]');
    await waitingRow.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toContainText('Back to Inbox');
    await page.mouse.click(10, 10);

    await page.locator('.sidebar-tab', { hasText: 'Done' }).click();
    const doneRow = page.locator('#pr-file-list .pr-file-item[data-item-id="102"]');
    await doneRow.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toContainText('Back to Inbox');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Back to Inbox' }).click();

    await expect(page.locator('#pr-file-list')).toContainText('No done items.');
    await page.locator('.sidebar-tab', { hasText: 'Inbox' }).click();
    await expect(page.locator('#pr-file-list')).toContainText('Answer triage email');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.action === 'item_state' && entry?.payload?.state === 'inbox');
    }).toBe(true);
  });

  test('partial swipe cancels cleanly, rapid swipes remain stable, and empty inbox stays inert', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);
    await openInbox(page);
    await setInboxItems(page, [parserInboxItem, privateEmailInboxItem]);

    const rowA = '#pr-file-list .pr-file-item[data-item-id="101"]';
    await touchPhase(page, rowA, 'start');
    await touchPhase(page, rowA, 'move', 20, 0);
    await touchPhase(page, rowA, 'end', 20, 0);
    await expect(page.locator(rowA)).toBeVisible();

    await touchPhase(page, rowA, 'start');
    await touchPhase(page, rowA, 'move', 90, 0);
    await touchPhase(page, rowA, 'end', 90, 0);
    const rowB = '#pr-file-list .pr-file-item[data-item-id="102"]';
    await touchPhase(page, rowB, 'start');
    await touchPhase(page, rowB, 'move', 180, 0);
    await touchPhase(page, rowB, 'end', 180, 0);

    await setInboxItems(page, []);
    await expect(page.locator('#item-sidebar-menu')).toBeHidden();
  });
});
