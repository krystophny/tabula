// Slopshell Playwright harness — fetch routes for items, contexts, actors,
// runtime workspaces fallback, /api/files passthrough, and remaining domain routes.
__harnessRouteHandlers.push(async function harnessRouteDomain(u, opts) {
      if (u.includes('/api/items/counts')) {
        const itemData = window.__itemSidebarData || defaultItemSidebarData();
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const countFor = (key) => Array.isArray(itemData[key])
          ? itemData[key].filter((item) => matchesItemFilters(item, sphere, filters)).length
          : 0;
        const counts = {
          inbox: countFor('inbox'),
          next: countFor('next'),
          waiting: countFor('waiting'),
          deferred: countFor('deferred'),
          someday: countFor('someday'),
          review: countFor('review'),
          done: countFor('done'),
        };
        const sectionFixture = window.__itemSidebarSectionCounts && typeof window.__itemSidebarSectionCounts === 'object'
          ? window.__itemSidebarSectionCounts
          : { project_items_open: 0, people_open: 0, drift_review: 0, dedup_review: 0, recent_meetings: 0 };
        const sections = {
          project_items_open: Math.max(0, Number(sectionFixture.project_items_open || 0) || 0),
          people_open: Math.max(0, Number(sectionFixture.people_open || 0) || 0),
          drift_review: Math.max(0, Number(sectionFixture.drift_review || 0) || 0),
          dedup_review: Math.max(0, Number(sectionFixture.dedup_review || 0) || 0),
          recent_meetings: Math.max(0, Number(sectionFixture.recent_meetings || 0) || 0),
        };
        const delayMs = nextItemSidebarResponseDelay('counts');
        if (delayMs > 0) await sleep(delayMs);
        return new Response(JSON.stringify({ ok: true, counts, sections }), { status: 200 });
      }
      if (/\/api\/items\/people\/\d+(?:\?|$)/.test(u)) {
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const match = u.match(/\/api\/items\/people\/(\d+)(?:\?|$)/);
        const person = personDashboardForActor(Number(match?.[1] || 0), sphere, filters);
        if (!person) return new Response('person not found', { status: 404 });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'person_dashboard',
          method: opts?.method || 'GET',
          url: u,
          payload: { actor_id: Number(match?.[1] || 0) },
        });
        return new Response(JSON.stringify({ ok: true, person }), { status: 200 });
      }
      if (u.includes('/api/items/people')) {
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const people = personDashboardRows(sphere, filters);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'people_dashboard',
          method: opts?.method || 'GET',
          url: u,
          payload: { count: people.length },
        });
        return new Response(JSON.stringify({ ok: true, people, total: people.length }), { status: 200 });
      }
      if (u.includes('/api/items/projects')) {
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const projectItems = projectItemReviewRows(sphere, filters);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_item_review',
          method: opts?.method || 'GET',
          url: u,
          payload: { count: projectItems.length },
        });
        return new Response(JSON.stringify({
          ok: true,
          project_items: projectItems,
          total: projectItems.length,
          stalled: projectItems.filter((row) => Boolean(row?.health?.stalled)).length,
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces')) {
        const workspaces = Array.isArray(window.__itemSidebarWorkspaces) ? window.__itemSidebarWorkspaces : defaultItemSidebarWorkspaces();
        const sphere = requestedSphere(u);
        const filtered = workspaces.filter((workspace) => !sphere || String(workspace?.sphere || '').trim().toLowerCase() === sphere);
        return new Response(JSON.stringify({ ok: true, workspaces: filtered }), { status: 200 });
      }
      if (u.includes('/api/contexts')) {
        const contexts = Array.isArray(window.__itemSidebarContexts) ? window.__itemSidebarContexts : defaultItemSidebarContexts();
        return new Response(JSON.stringify({ ok: true, contexts }), { status: 200 });
      }
      if (/\/api\/items(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const stateKey = ['waiting', 'someday', 'done'].includes(String(body.state || '').trim().toLowerCase())
          ? String(body.state || '').trim().toLowerCase()
          : 'inbox';
        const artifactID = Number(body.artifact_id || 0);
        const artifacts = window.__itemSidebarArtifacts && typeof window.__itemSidebarArtifacts === 'object'
          ? window.__itemSidebarArtifacts
          : {};
        const artifact = artifacts[String(artifactID)] || artifacts[artifactID] || null;
        const itemID = Number(window.__itemSidebarNextItemID || 1000);
        window.__itemSidebarNextItemID = itemID + 1;
        const item = prependItemSidebarEntry({
          id: itemID,
          title: String(body.title || 'Follow up').trim() || 'Follow up',
          state: stateKey,
          sphere: normalizeHarnessSphere(body.sphere),
          artifact_id: artifactID > 0 ? artifactID : 0,
          source: '',
          source_ref: '',
          artifact_title: String(artifact?.title || '').trim(),
          artifact_kind: String(artifact?.kind || '').trim(),
          actor_name: '',
          created_at: '2026-03-10 10:20:00',
          updated_at: '2026-03-10 10:20:00',
        }, stateKey);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_create',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({ ok: true, item }), { status: 201 });
      }
      if (/\/api\/items\/\d+\/gesture(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/gesture(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const action = String(body.action || '').trim().toLowerCase();
        const actors = Array.isArray(window.__itemSidebarActors) ? window.__itemSidebarActors : [];
        const existingRows = itemSidebarStateKeys()
          .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
        const existing = existingRows.find((entry) => Number(entry?.id || 0) === itemID) || null;
        const undoSnapshot = existing ? {
          state: String(existing.state || 'inbox'),
          actor_id: existing.actor_id == null ? null : Number(existing.actor_id || 0),
          visible_after: existing.visible_after == null ? null : String(existing.visible_after || ''),
          follow_up_at: existing.follow_up_at == null ? null : String(existing.follow_up_at || ''),
        } : { state: 'inbox' };
        let item = null;
        let dropMode = '';
        if (action === 'complete') {
          item = moveItemSidebarEntry(itemID, 'done');
        } else if (action === 'drop') {
          dropMode = 'local_overlay';
          item = moveItemSidebarEntry(itemID, 'done');
        } else if (action === 'defer') {
          const followUpAt = String(body.follow_up_at || '');
          item = moveItemSidebarEntry(itemID, 'deferred', { visible_after: followUpAt, follow_up_at: followUpAt });
        } else if (action === 'delegate') {
          const actor = actors.find((entry) => Number(entry?.id || 0) === Number(body.actor_id || 0));
          item = moveItemSidebarEntry(itemID, 'waiting', { actor_name: String(actor?.name || '') });
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_gesture',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({
          ok: true,
          data: {
            item,
            action,
            drop_mode: dropMode,
            email_sync_back: false,
            undo: undoSnapshot,
          },
        }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/gesture\/undo(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/gesture\/undo(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const undo = body.undo && typeof body.undo === 'object' ? body.undo : {};
        const restoredState = String(undo.state || 'inbox');
        const item = moveItemSidebarEntry(itemID, restoredState, {
          actor_id: undo.actor_id == null ? null : Number(undo.actor_id || 0),
          visible_after: undo.visible_after == null ? null : String(undo.visible_after || ''),
          follow_up_at: undo.follow_up_at == null ? null : String(undo.follow_up_at || ''),
        });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_gesture_undo',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, data: { item } }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/triage(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/triage(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const action = String(body.action || '').trim().toLowerCase();
        const actors = Array.isArray(window.__itemSidebarActors) ? window.__itemSidebarActors : [];
        let item = null;
        if (action === 'next') {
          item = moveItemSidebarEntry(itemID, 'next');
        } else if (action === 'done') {
          item = moveItemSidebarEntry(itemID, 'done');
        } else if (action === 'later') {
          item = moveItemSidebarEntry(itemID, 'deferred', { visible_after: String(body.visible_after || '') });
        } else if (action === 'delegate') {
          const actor = actors.find((entry) => Number(entry?.id || 0) === Number(body.actor_id || 0));
          item = moveItemSidebarEntry(itemID, 'waiting', { actor_name: String(actor?.name || '') });
        } else if (action === 'someday') {
          item = moveItemSidebarEntry(itemID, 'someday');
        } else if (action === 'delete') {
          item = deleteItemSidebarEntry(itemID);
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_triage',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        if (action === 'delete') {
          return new Response(JSON.stringify({ ok: true, deleted: true, item_id: itemID }), { status: 200 });
        }
        return new Response(JSON.stringify({ ok: true, item }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/project-item-link(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/project-item-link(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const projectItemID = Number(body.project_item_id || 0);
        const rows = itemSidebarStateKeys()
          .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
        const project = rows.find((entry) => Number(entry?.id || 0) === projectItemID) || null;
        if (!project || String(project?.kind || '').trim().toLowerCase() !== 'project' || itemID === projectItemID) {
          return new Response('project item not found', { status: 400 });
        }
        const child = patchItemSidebarEntry(itemID, { project_item_id: projectItemID });
        if (!child) {
          return new Response('project item not found', { status: 400 });
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_project_item_link',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({
          ok: true,
          item: child,
          project_item_id: projectItemID,
          links: [{
            parent_item_id: projectItemID,
            child_item_id: itemID,
            role: String(body.role || 'next_action').trim() || 'next_action',
          }],
          health: { stalled: true },
        }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/dispatch-review(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/dispatch-review(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const target = String(body.target || '').trim().toLowerCase();
        const reviewer = String(body.reviewer || '').trim();
        const email = String(body.email || '').trim();
        if (!['agent', 'github', 'email'].includes(target)) {
          return new Response('invalid review target', { status: 400 });
        }
        if (target === 'github' && !reviewer) {
          return new Response('reviewer is required', { status: 400 });
        }
        if (target === 'email' && !email) {
          return new Response('email is required', { status: 400 });
        }
        const item = moveItemSidebarEntry(itemID, 'waiting', {
          review_target: target,
          reviewer: target === 'github' ? reviewer : (target === 'email' ? email : ''),
          reviewed_at: '2026-03-10T10:15:00Z',
        });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dispatch_review',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, item }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/workspace(?:\?|$)/.test(u) && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/workspace(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const workspaceID = body.workspace_id == null ? null : Number(body.workspace_id || 0);
        const workspaces = Array.isArray(window.__itemSidebarWorkspaces) ? window.__itemSidebarWorkspaces : defaultItemSidebarWorkspaces();
        if (workspaceID !== null && !workspaces.some((workspace) => Number(workspace?.id || 0) === workspaceID)) {
          return new Response('workspace not found', { status: 400 });
        }
        const existingRows = ['inbox', 'waiting', 'someday', 'done']
          .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
        const existing = existingRows.find((entry) => Number(entry?.id || 0) === itemID) || null;
        const previousWorkspaceID = existing && existing.workspace_id != null ? Number(existing.workspace_id || 0) : null;
        const item = patchItemSidebarEntry(itemID, { workspace_id: workspaceID });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_workspace',
          method: opts?.method || 'PUT',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        const workspace = workspaceID === null
          ? null
          : workspaces.find((entry) => Number(entry?.id || 0) === workspaceID) || null;
        if (workspace?.sphere) {
          item.sphere = String(workspace.sphere).trim().toLowerCase();
        }
        const warning = previousWorkspaceID === null || previousWorkspaceID === workspaceID
          ? ''
          : 'Artifact link kept: the referenced file still points into the previous workspace.';
        return new Response(JSON.stringify({ ok: true, item, warning }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/project(?:\?|$)/.test(u) && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/project(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const workspaceID = body.workspace_id == null ? '' : String(body.workspace_id || '').trim();
        if (workspaceID && !harnessProjects.some((project) => String(project.id || '').trim() === workspaceID)) {
          return new Response('project not found', { status: 400 });
        }
        const item = patchItemSidebarEntry(itemID, { workspace_id: workspaceID });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_project',
          method: opts?.method || 'PUT',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, item }), { status: 200 });
      }
      if (/\/api\/items\/\d+(?:\?|$)/.test(u) && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const patch = {};
        if (typeof body?.sphere === 'string') {
          patch.sphere = String(body.sphere).trim().toLowerCase() === 'work' ? 'work' : 'private';
        }
        const item = patchItemSidebarEntry(itemID, patch);
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_update',
          method: opts?.method || 'PUT',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({ ok: true, item }), { status: 200 });
      }
      if (/\/api\/items\/\d+\/state(?:\?|$)/.test(u) && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/items\/(\d+)\/state(?:\?|$)/);
        const itemID = Number(match?.[1] || 0);
        const nextState = String(body.state || '').trim().toLowerCase();
        if (!['inbox', 'next', 'waiting', 'deferred', 'someday', 'review', 'done'].includes(nextState)) {
          return new Response('invalid state', { status: 400 });
        }
        const item = moveItemSidebarEntry(itemID, nextState);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_state',
          method: opts?.method || 'PUT',
          url: u,
          payload: body,
        });
        if (!item) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, item }), { status: 200 });
      }
      if (u.includes('/api/actors')) {
        const actors = Array.isArray(window.__itemSidebarActors) ? window.__itemSidebarActors : [];
        return new Response(JSON.stringify({ ok: true, actors }), { status: 200 });
      }
      if (/\/api\/mail\/drafts\/reply-all(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const draft = createHarnessReplyAllDraft(Number(body.item_id || 0));
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_reply_all',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!draft) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft }), { status: 201 });
      }
      if (/\/api\/mail\/drafts\/forward(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const draft = createHarnessForwardDraft(Number(body.item_id || 0));
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_forward',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!draft) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft }), { status: 201 });
      }
      if (/\/api\/mail\/drafts\/reply(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const draft = createHarnessReplyDraft(Number(body.item_id || 0));
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_reply',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        if (!draft) {
          return new Response('item not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft }), { status: 201 });
      }
      if (/\/api\/mail\/drafts\/\d+\/send(?:\?|$)/.test(u) && opts?.method === 'POST') {
        const match = u.match(/\/api\/mail\/drafts\/(\d+)\/send(?:\?|$)/);
        const artifactID = Number(match?.[1] || 0);
        const draft = updateHarnessMailDraft(artifactID, { status: 'sent' });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_send',
          method: opts?.method || 'POST',
          url: u,
          payload: { artifact_id: artifactID },
        });
        if (!draft) {
          return new Response('draft not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft }), { status: 200 });
      }
      if (/\/api\/mail\/drafts\/\d+(?:\?|$)/.test(u) && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/mail\/drafts\/(\d+)(?:\?|$)/);
        const artifactID = Number(match?.[1] || 0);
        const draft = updateHarnessMailDraft(artifactID, {
          to: normalizeMailDraftAddresses(body.to),
          cc: normalizeMailDraftAddresses(body.cc),
          bcc: normalizeMailDraftAddresses(body.bcc),
          subject: String(body.subject || '').trim(),
          body: String(body.body || ''),
        });
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_update',
          method: opts?.method || 'PUT',
          url: u,
          payload: body,
        });
        if (!draft) {
          return new Response('draft not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft }), { status: 200 });
      }
      if (/\/api\/mail\/drafts\/\d+(?:\?|$)/.test(u)) {
        const match = u.match(/\/api\/mail\/drafts\/(\d+)(?:\?|$)/);
        const artifactID = Number(match?.[1] || 0);
        const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
        const draft = drafts[String(artifactID)] || drafts[artifactID];
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_get',
          method: opts?.method || 'GET',
          url: u,
          payload: { artifact_id: artifactID },
        });
        if (!draft) {
          return new Response('draft not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, draft: cloneMailDraft(draft) }), { status: 200 });
      }
      if (/\/api\/mail\/drafts(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const draft = createHarnessDraftFromRequest(body);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'mail_draft_create',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({ ok: true, draft }), { status: 201 });
      }
      if (/\/api\/artifacts\/\d+\/materialize(?:\?|$)/.test(u) && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const match = u.match(/\/api\/artifacts\/(\d+)\/materialize(?:\?|$)/);
        const artifactID = Number(match?.[1] || 0);
        const artifacts = window.__itemSidebarArtifacts && typeof window.__itemSidebarArtifacts === 'object'
          ? window.__itemSidebarArtifacts
          : {};
        const artifact = artifacts[String(artifactID)] || artifacts[artifactID];
        if (!artifact) {
          return new Response('artifact not found', { status: 404 });
        }
        const relativePath = `.slopshell/artifacts/materialized/${artifactID}.md`;
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'artifact_materialize',
          method: opts?.method || 'POST',
          url: u,
          payload: { ...body, artifact_id: artifactID, relative_path: relativePath },
        });
        return new Response(JSON.stringify({
          ok: true,
          artifact: { ...artifact, ref_path: `/tmp/test/${relativePath}` },
          relative_path: relativePath,
        }), { status: 200 });
      }
      if (/\/api\/artifacts\/\d+(?:\?|$)/.test(u)) {
        const match = u.match(/\/api\/artifacts\/(\d+)(?:\?|$)/);
        const artifactID = Number(match?.[1] || 0);
        const artifacts = window.__itemSidebarArtifacts && typeof window.__itemSidebarArtifacts === 'object'
          ? window.__itemSidebarArtifacts
          : {};
        const artifact = artifacts[String(artifactID)] || artifacts[artifactID];
        if (!artifact) {
          return new Response('artifact not found', { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true, artifact }), { status: 200 });
      }
      if (u.includes('/api/items/inbox') || u.includes('/api/items/next') || u.includes('/api/items/waiting') || u.includes('/api/items/deferred') || u.includes('/api/items/someday') || u.includes('/api/items/review') || u.includes('/api/items/done')) {
        const itemData = window.__itemSidebarData || defaultItemSidebarData();
        let key = 'inbox';
        if (u.includes('/api/items/next')) key = 'next';
        if (u.includes('/api/items/waiting')) key = 'waiting';
        if (u.includes('/api/items/deferred')) key = 'deferred';
        if (u.includes('/api/items/someday')) key = 'someday';
        if (u.includes('/api/items/review')) key = 'review';
        if (u.includes('/api/items/done')) key = 'done';
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const items = cloneItemSidebarEntries(itemData[key])
          .filter((item) => matchesItemFilters(item, sphere, filters));
        const delayMs = nextItemSidebarResponseDelay(key);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_list',
          method: opts?.method || 'GET',
          url: u,
          payload: { view: key, delay_ms: delayMs },
        });
        if (delayMs > 0) await sleep(delayMs);
        return new Response(JSON.stringify({ ok: true, items }), { status: 200 });
      }
      if (/\/api\/items(?:\?|$)/.test(u) && (!opts?.method || opts.method === 'GET')) {
        const itemData = window.__itemSidebarData || defaultItemSidebarData();
        const sphere = requestedSphere(u);
        const filters = requestedItemFilters(u);
        const items = itemSidebarStateKeys()
          .flatMap((key) => cloneItemSidebarEntries(itemData[key]))
          .filter((item) => matchesItemFilters(item, sphere, filters));
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'item_list_all',
          method: opts?.method || 'GET',
          url: u,
          payload: { count: items.length },
        });
        return new Response(JSON.stringify({ ok: true, items }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces')) {
        const workspaces = harnessProjects.map((project) => cloneProject(project.id));
        return new Response(JSON.stringify({
          workspaces,
          default_workspace_id: 'test',
          active_workspace_id: activeWorkspaceId,
        }), { status: 200 });
      }
      if (u.includes('/api/ink/submit') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'ink_submit',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: String(body.workspace_id || activeWorkspaceId),
          ink_svg_path: '.slopshell/artifacts/ink/test-ink.svg',
          ink_png_path: '.slopshell/artifacts/ink/test-ink.png',
          summary_path: '.slopshell/artifacts/ink/test-ink.md',
          revision_manifest_path: '.slopshell/revisions/readme-md/manifest.json',
          revision_history_path: '.slopshell/revisions/readme-md/history.md',
        }), { status: 200 });
      }
      if (u.includes('/api/scan/upload') && opts?.method === 'POST') {
        const form = opts?.body instanceof FormData ? opts.body : null;
        const file = form?.get('file');
        const filename = file instanceof File ? file.name : '';
        const bytes = file instanceof Blob ? (await file.arrayBuffer()).byteLength : 0;
        const projectId = String(form?.get('workspace_id') || activeWorkspaceId).trim();
        const itemId = Number(form?.get('item_id') || 0);
        const artifactId = Number(form?.get('artifact_id') || 0);
        const request = { workspace_id: projectId, item_id: itemId, artifact_id: artifactId, filename, bytes };
        window.__scanUploadRequests.push(request);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'scan_upload',
          method: opts?.method || 'POST',
          url: u,
          payload: request,
        });
        return new Response(JSON.stringify({
          ...window.__scanUploadResponse,
          workspace_id: projectId || window.__scanUploadResponse.workspace_id,
          item_id: itemId || window.__scanUploadResponse.item_id,
          artifact_id: artifactId || window.__scanUploadResponse.artifact_id,
        }), { status: 201 });
      }
      if (u.includes('/api/scan/confirm') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__scanConfirmRequests.push(body);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'scan_confirm',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({
          ...window.__scanConfirmResponse,
          workspace_id: String(body.workspace_id || window.__scanConfirmResponse.workspace_id || activeWorkspaceId),
          item_id: Number(body.item_id || window.__scanConfirmResponse.item_id || 0),
          artifact_id: Number(body.artifact_id || window.__scanConfirmResponse.artifact_id || 0),
          scan_artifact_id: Number(body.scan_artifact_id || window.__scanConfirmResponse.scan_artifact_id || 0),
          annotations: Array.isArray(body.annotations) ? body.annotations : window.__scanConfirmResponse.annotations,
        }), { status: 201 });
      }
      if (u.includes('/api/participant/config') && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__participantConfig = {
          ...window.__participantConfig,
          companion_enabled: Object.prototype.hasOwnProperty.call(body, 'companion_enabled')
            ? Boolean(body.companion_enabled)
            : Boolean(window.__participantConfig.companion_enabled),
          language: String(body.language || window.__participantConfig.language || 'en'),
          max_segment_duration_ms: Number(body.max_segment_duration_ms || window.__participantConfig.max_segment_duration_ms || 30000),
          session_ram_cap_mb: Number(body.session_ram_cap_mb || window.__participantConfig.session_ram_cap_mb || 64),
          stt_model: String(body.stt_model || window.__participantConfig.stt_model || 'whisper-1'),
          idle_surface: String(body.idle_surface || window.__participantConfig.idle_surface || 'robot'),
          audio_persistence: 'none',
          capture_source: 'microphone',
        };
        return new Response(JSON.stringify(window.__participantConfig), { status: 200 });
      }
      if (u.includes('/api/participant/config')) {
        return new Response(JSON.stringify(window.__participantConfig), { status: 200 });
      }
      if (u.includes('/api/participant/status')) {
        return new Response(JSON.stringify(window.__participantStatus || {
          ok: true,
          active_sessions: 0,
          directed_speech_gate: { decision: 'disabled', reason: 'harness' },
          interaction_policy: { decision: 'disabled', reason: 'harness' },
        }), { status: 200 });
      }
      if (u.includes('/api/participant/sessions') && u.includes('/transcript')) {
        return new Response(JSON.stringify({ ok: true, segments: [] }), { status: 200 });
      }
      if (u.includes('/api/participant/sessions') && u.includes('/search')) {
        return new Response(JSON.stringify({ ok: true, segments: [] }), { status: 200 });
      }
      if (u.includes('/api/participant/sessions') && u.includes('/export')) {
        return new Response(JSON.stringify({ ok: true, session: {}, segments: [] }), { status: 200 });
      }
      if (u.includes('/api/participant/sessions')) {
        return new Response(JSON.stringify({ ok: true, sessions: [] }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/cancel')) {
        const response = cancelResponsesQueue.length > 0
          ? { ...DEFAULT_CANCEL_RESPONSE, ...cancelResponsesQueue.shift() }
          : { ...DEFAULT_CANCEL_RESPONSE };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'cancel',
          method: opts?.method || 'GET',
          url: u,
          payload: response,
        });
        return new Response(JSON.stringify(response), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/activity')) {
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'activity',
          method: opts?.method || 'GET',
          url: u,
          payload: activityResponse,
        });
        return new Response(JSON.stringify(activityResponse), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/dictation/start') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const requestedTargetKind = String(body.target_kind || '').trim().toLowerCase();
        const targetKind = ['document_section', 'email_draft', 'email_reply', 'review_comment'].includes(requestedTargetKind)
          ? requestedTargetKind
          : dictationTargetForPrompt(body.prompt, body.artifact_title);
        dictationState = {
          active: true,
          target_kind: targetKind,
          target_label: dictationLabel(targetKind),
          prompt: String(body.prompt || '').trim(),
          artifact_title: String(body.artifact_title || '').trim(),
          transcript: '',
          draft_text: '',
          scratch_path: '',
        };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dictation_start',
          method: opts?.method || 'POST',
          url: u,
          payload: { ...dictationState },
        });
        return new Response(JSON.stringify({ ok: true, dictation: dictationState }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/dictation/append') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const chunk = String(body.text || '').trim();
        dictationState.transcript = [dictationState.transcript, chunk].filter(Boolean).join('\n\n').trim();
        dictationState.draft_text = dictationDraftForState(dictationState);
        if (!dictationState.scratch_path) {
          dictationState.scratch_path = '.slopshell/artifacts/tmp/dictation-harness.md';
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dictation_append',
          method: opts?.method || 'POST',
          url: u,
          payload: { ...dictationState, chunk },
        });
        return new Response(JSON.stringify({ ok: true, dictation: dictationState }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/dictation/draft') && opts?.method === 'PUT') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        dictationState.draft_text = String(body.draft_text || '').trim();
        if (!dictationState.scratch_path) {
          dictationState.scratch_path = '.slopshell/artifacts/tmp/dictation-harness.md';
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dictation_draft',
          method: opts?.method || 'PUT',
          url: u,
          payload: { ...dictationState },
        });
        return new Response(JSON.stringify({ ok: true, dictation: dictationState }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/dictation') && opts?.method === 'DELETE') {
        dictationState = { ...dictationState, active: false };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dictation_stop',
          method: opts?.method || 'DELETE',
          url: u,
          payload: { ...dictationState },
        });
        return new Response(JSON.stringify({ ok: true, dictation: dictationState }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/dictation')) {
        return new Response(JSON.stringify({ ok: true, dictation: dictationState }), { status: 200 });
      }
      if (u.includes('/api/chat/sessions') && u.includes('/commands') && opts?.method === 'POST') {
        let body = {};
        try {
          body = JSON.parse(String(opts?.body || '{}'));
        } catch (_) {
          body = {};
        }
        const command = String(body.command || '').trim();
        window.__harnessLog.push({ type: 'command_sent', command });
        const prMatch = command.match(/^\/?pr\s+(\d+)$/i);
        if (prMatch) {
          const prNumber = Number(prMatch[1]);
          const app = window._slopshellApp;
          window.setTimeout(() => {
            const canvasWs = app?.getState?.().canvasWs;
            if (canvasWs && typeof canvasWs.injectEvent === 'function') {
              canvasWs.injectEvent({
                kind: 'text_artifact',
                event_id: `evt-pr-command-${prNumber}`,
                title: `.slopshell/artifacts/pr/pr-${prNumber}.diff`,
                text: mockPrDiff(prNumber),
              });
            }
          }, 0);
          return new Response(JSON.stringify({
            ok: true,
            kind: 'command',
            result: {
              name: 'pr',
              pr_number: prNumber,
              pr_title: `Harness PR ${prNumber}`,
              pr_url: `https://github.com/owner/repo/pull/${prNumber}`,
              files_changed: 1,
              message: `Loaded PR #${prNumber}: Harness PR ${prNumber} (1 file).`,
            },
          }), { status: 200 });
        }
        return new Response(JSON.stringify({
          ok: true,
          kind: 'command',
          result: { name: 'noop', message: 'Harness command' },
        }), { status: 200 });
      }
      if (u.includes('/api/chat/') && u.includes('/messages') && opts?.method === 'POST') {
        const signal = opts?.signal;
        if (messagePostDelayMs > 0) {
          await new Promise((resolve, reject) => {
            const timer = window.setTimeout(() => {
              if (signal && typeof signal.removeEventListener === 'function') {
                signal.removeEventListener('abort', onAbort);
              }
              resolve();
            }, messagePostDelayMs);
            const onAbort = () => {
              window.clearTimeout(timer);
              if (signal && typeof signal.removeEventListener === 'function') {
                signal.removeEventListener('abort', onAbort);
              }
              reject(new DOMException('aborted', 'AbortError'));
            };
            if (signal && typeof signal.addEventListener === 'function') {
              signal.addEventListener('abort', onAbort, { once: true });
            }
          });
        }
        if (signal && signal.aborted) {
          throw new DOMException('aborted', 'AbortError');
        }
        try {
          const body = JSON.parse(opts.body);
          window.__harnessLog.push({ type: 'message_sent', text: body.text, cursor: body.cursor || null });
        } catch (_) {}
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      if (u.includes('/api/stt/transcribe') && opts?.method === 'POST') {
        let mimeType = '';
        let bytes = 0;
        const body = opts?.body;
        if (body && typeof body.get === 'function') {
          try {
            mimeType = String(body.get('mime_type') || '').trim();
          } catch (_) {}
          try {
            const file = body.get('file');
            bytes = Number(file?.size || 0);
            if (!mimeType) mimeType = String(file?.type || '').trim();
          } catch (_) {}
        }
        if (!mimeType) mimeType = 'application/octet-stream';
        window.__harnessLog.push({ type: 'api_fetch', action: 'stt_transcribe', mime_type: mimeType, bytes });
        const payload = sttTranscribePayload && typeof sttTranscribePayload === 'object'
          ? JSON.stringify(sttTranscribePayload)
          : '{}';
        return new Response(payload, {
          status: sttTranscribeStatus,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (u.includes('/api/chat/')) {
        return new Response(JSON.stringify({ messages: [], turns: [] }), { status: 200 });
      }
      if (u.includes('/api/assistant/')) {
        return new Response(JSON.stringify({ active: 0, queued: 0 }), { status: 200 });
      }
      if (u.includes('/api/runtime')) {
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'runtime_meta',
          method: opts?.method || 'GET',
          url: u,
        });
        return new Response(JSON.stringify({ ...runtimeState }), { status: 200 });
      }
      if (u.includes('/api/canvas/')) {
        return new Response(JSON.stringify({}), { status: 200 });
      }
      if (u.includes('/api/files/')) {
        const pathPart = u.split('/api/files/')[1] || '';
        const slash = pathPart.indexOf('/');
        const encodedPath = slash >= 0 ? pathPart.slice(slash + 1) : '';
        const filePath = decodeURIComponent(encodedPath || '').trim() || 'file';
        return new Response(`# Harness file: ${filePath}\n`, {
          status: 200,
          headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
        });
      }
    });
