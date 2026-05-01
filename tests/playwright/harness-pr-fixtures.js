// Slopshell Playwright harness — pull request mock diff and project clone helpers.
    const mockPrDiff = (prNumber) => [
      'diff --git a/src/review.js b/src/review.js',
      'index 1111111..2222222 100644',
      '--- a/src/review.js',
      '+++ b/src/review.js',
      `@@ -1 +1 @@`,
      `-console.log("before ${prNumber}");`,
      `+console.log("after ${prNumber}");`,
    ].join('\n');
    window.__itemSidebarData = defaultItemSidebarData();
    window.__itemSidebarActors = defaultItemSidebarActors();
    window.__itemSidebarWorkspaces = defaultItemSidebarWorkspaces();
    window.__itemSidebarContexts = defaultItemSidebarContexts();
    window.__itemSidebarArtifacts = defaultItemSidebarArtifacts();
    window.__mailDrafts = {};
    window.__mailDraftCounters = {
      artifact_id: 950,
      item_id: 950,
      remote_id: 1,
    };
    window.__itemSidebarNextItemID = 1000;
    window.__meetingSummaryProposals = defaultMeetingSummaryProposals();
    window.__itemSidebarResponseDelays = {
      inbox: [],
      waiting: [],
      someday: [],
      done: [],
      counts: [],
    };
    window.__scanUploadRequests = [];
    window.__scanConfirmRequests = [];
    window.__scanUploadResponse = {
      ok: true,
      workspace_id: 'test',
      item_id: 0,
      artifact_id: 0,
      scan_artifact: { id: 990, kind: 'image', title: 'Scanned annotations' },
      summary_path: '.slopshell/artifacts/scans/test-upload.md',
      annotations: [
        { content: 'check null case', anchor_text: 'Line two', line: 2, confidence: 0.9 },
      ],
    };
    window.__scanConfirmResponse = {
      ok: true,
      workspace_id: 'test',
      item_id: 0,
      artifact_id: 0,
      scan_artifact_id: 990,
      review_artifact: { id: 991, kind: 'annotation', title: 'Reviewed annotations' },
      summary_path: '.slopshell/artifacts/scans/test-confirm.md',
      annotations: [
        { content: 'check null case', anchor_text: 'Line two', line: 2, confidence: 0.9 },
      ],
    };
    function normalizeHarnessSphere(value) {
      const sphere = String(value || '').trim().toLowerCase();
      if (sphere === 'work' || sphere === 'private') return sphere;
      return String(runtimeState.active_sphere || 'private').trim().toLowerCase() === 'work' ? 'work' : 'private';
    }
    function normalizeHarnessItemSidebarEntry(entry) {
      if (!entry || typeof entry !== 'object') return {};
      return {
        ...entry,
        sphere: normalizeHarnessSphere(entry.sphere),
      };
    }
    function normalizeHarnessItemSidebarEntries(entries) {
      return Array.isArray(entries) ? entries.map((entry) => normalizeHarnessItemSidebarEntry(entry)) : [];
    }
    function itemSidebarStateKeys() {
      return ['inbox', 'next', 'waiting', 'deferred', 'someday', 'review', 'done'];
    }
    window.__setItemSidebarData = (next) => {
      const incoming = next && typeof next === 'object' ? next : {};
      const data = {
        ...defaultItemSidebarData(),
        ...incoming,
      };
      window.__itemSidebarData = itemSidebarStateKeys().reduce((acc, key) => {
        acc[key] = normalizeHarnessItemSidebarEntries(data[key]);
        return acc;
      }, {});
    };
    window.__setItemSidebarActors = (next) => {
      window.__itemSidebarActors = Array.isArray(next)
        ? next.map((actor) => ({ ...actor }))
        : defaultItemSidebarActors();
    };
    window.__setItemSidebarWorkspaces = (next) => {
      window.__itemSidebarWorkspaces = Array.isArray(next)
        ? next.map((workspace) => ({ ...workspace }))
        : defaultItemSidebarWorkspaces();
    };
    window.__setItemSidebarContexts = (next) => {
      window.__itemSidebarContexts = Array.isArray(next)
        ? next.map((entry) => ({ ...entry }))
        : defaultItemSidebarContexts();
    };
    window.__setItemSidebarArtifacts = (next) => {
      const source = next && typeof next === 'object' ? next : {};
      window.__itemSidebarArtifacts = Object.keys(source).reduce((acc, key) => {
        acc[key] = { ...source[key] };
        return acc;
      }, {});
    };
    window.__setMailDrafts = (next) => {
      const source = next && typeof next === 'object' ? next : {};
      window.__mailDrafts = Object.keys(source).reduce((acc, key) => {
        acc[key] = JSON.parse(JSON.stringify(source[key]));
        return acc;
      }, {});
    };
    window.__setMeetingSummaryProposals = (next) => {
      window.__meetingSummaryProposals = Array.isArray(next)
        ? next.map((entry, index) => ({ index, ...entry }))
        : defaultMeetingSummaryProposals();
    };
    window.__queueItemSidebarResponseDelay = (view, delayMs) => {
      const key = String(view || '').trim().toLowerCase();
      if (![...itemSidebarStateKeys(), 'counts'].includes(key)) return;
      const queue = window.__itemSidebarResponseDelays && typeof window.__itemSidebarResponseDelays === 'object'
        ? window.__itemSidebarResponseDelays
        : {};
      if (!Array.isArray(queue[key])) queue[key] = [];
      queue[key].push(Math.max(0, Number(delayMs) || 0));
      window.__itemSidebarResponseDelays = queue;
    };
    window.__setScanUploadResponse = (next) => {
      const incoming = next && typeof next === 'object' ? next : {};
      window.__scanUploadResponse = {
        ...window.__scanUploadResponse,
        ...incoming,
      };
    };
    window.__setScanConfirmResponse = (next) => {
      const incoming = next && typeof next === 'object' ? next : {};
      window.__scanConfirmResponse = {
        ...window.__scanConfirmResponse,
        ...incoming,
      };
    };
    function cloneItemSidebarEntry(entry) {
      return normalizeHarnessItemSidebarEntry(entry);
    }
    function cloneItemSidebarEntries(entries) {
      return Array.isArray(entries) ? entries.map((entry) => cloneItemSidebarEntry(entry)) : [];
    }
    function nextItemSidebarResponseDelay(view) {
      const key = String(view || '').trim().toLowerCase();
      const queue = window.__itemSidebarResponseDelays && typeof window.__itemSidebarResponseDelays === 'object'
        ? window.__itemSidebarResponseDelays
        : {};
      if (!Array.isArray(queue[key]) || queue[key].length === 0) return 0;
      return Math.max(0, Number(queue[key].shift()) || 0);
    }
    function sleep(ms) {
      if (!Number.isFinite(Number(ms)) || Number(ms) <= 0) return Promise.resolve();
      return new Promise((resolve) => window.setTimeout(resolve, Number(ms)));
    }
    function moveItemSidebarEntry(itemID, nextState, patch = {}) {
      const data = window.__itemSidebarData || defaultItemSidebarData();
      let found = null;
      itemSidebarStateKeys().forEach((stateKey) => {
        const rows = Array.isArray(data[stateKey]) ? data[stateKey] : [];
        const index = rows.findIndex((entry) => Number(entry?.id || 0) === Number(itemID));
        if (index >= 0) {
          found = cloneItemSidebarEntry(rows[index]);
          rows.splice(index, 1);
          data[stateKey] = rows;
        }
      });
      if (!found) return null;
      const updated = {
        ...found,
        ...patch,
        state: nextState,
        updated_at: '2026-03-08 15:00:00',
      };
      if (!Array.isArray(data[nextState])) data[nextState] = [];
      data[nextState] = [normalizeHarnessItemSidebarEntry(updated)].concat(data[nextState]);
      window.__itemSidebarData = data;
      return normalizeHarnessItemSidebarEntry(updated);
    }
    function deleteItemSidebarEntry(itemID) {
      const data = window.__itemSidebarData || defaultItemSidebarData();
      let removed = null;
      itemSidebarStateKeys().forEach((stateKey) => {
        const rows = Array.isArray(data[stateKey]) ? data[stateKey] : [];
        const index = rows.findIndex((entry) => Number(entry?.id || 0) === Number(itemID));
        if (index >= 0) {
          removed = cloneItemSidebarEntry(rows[index]);
          rows.splice(index, 1);
          data[stateKey] = rows;
        }
      });
      window.__itemSidebarData = data;
      return removed;
    }
    function patchItemSidebarEntry(itemID, patch = {}) {
      const data = window.__itemSidebarData || defaultItemSidebarData();
      let updated = null;
      itemSidebarStateKeys().forEach((stateKey) => {
        const rows = Array.isArray(data[stateKey]) ? data[stateKey] : [];
        const index = rows.findIndex((entry) => Number(entry?.id || 0) === Number(itemID));
        if (index < 0) return;
        updated = {
          ...cloneItemSidebarEntry(rows[index]),
          ...patch,
          updated_at: '2026-03-08 15:00:00',
        };
        rows[index] = normalizeHarnessItemSidebarEntry(updated);
        data[stateKey] = rows;
      });
      window.__itemSidebarData = data;
      return updated ? normalizeHarnessItemSidebarEntry(updated) : null;
    }
    function prependItemSidebarEntry(entry, stateKey) {
      const data = window.__itemSidebarData || defaultItemSidebarData();
      const key = String(stateKey || 'inbox').trim().toLowerCase();
      const rows = Array.isArray(data[key]) ? data[key] : [];
      data[key] = [cloneItemSidebarEntry({ ...entry, state: key })].concat(rows);
      window.__itemSidebarData = data;
      return cloneItemSidebarEntry(entry);
    }
    function prependInboxItem(entry) {
      return prependItemSidebarEntry(entry, 'inbox');
    }
    function normalizeMailDraftAddresses(value) {
      if (Array.isArray(value)) {
        return value.map((entry) => String(entry || '').trim()).filter(Boolean);
      }
      return String(value || '')
        .split(',')
        .map((entry) => String(entry || '').trim())
        .filter(Boolean);
    }
    function firstMailAddress(value) {
      const match = String(value || '').match(/<([^>]+)>/);
      if (match && match[1]) return String(match[1]).trim().toLowerCase();
      return String(value || '').trim().toLowerCase();
    }
    function mailDraftTitle(subject) {
      const trimmed = String(subject || '').trim();
      return trimmed || 'Draft email';
    }
    function nextMailDraftCounter(key) {
      const counters = window.__mailDraftCounters && typeof window.__mailDraftCounters === 'object'
        ? window.__mailDraftCounters
        : { artifact_id: 950, item_id: 950, remote_id: 1 };
      const current = Math.max(1, Number(counters[key] || 1));
      counters[key] = current + 1;
      window.__mailDraftCounters = counters;
      return current;
    }
    function normalizeMailDraft(draft) {
      const artifactID = Math.max(1, Number(draft?.artifact_id || 0));
      const itemID = Math.max(1, Number(draft?.item_id || 0));
      const provider = String(draft?.provider || 'gmail').trim().toLowerCase() || 'gmail';
      const accountLabel = String(draft?.account_label || 'Private Gmail').trim() || 'Private Gmail';
      const subject = String(draft?.subject || '').trim();
      const status = String(draft?.status || 'draft').trim().toLowerCase() === 'sent' ? 'sent' : 'draft';
      const to = normalizeMailDraftAddresses(draft?.to);
      const cc = normalizeMailDraftAddresses(draft?.cc);
      const bcc = normalizeMailDraftAddresses(draft?.bcc);
      const normalized = {
        artifact_id: artifactID,
        item_id: itemID,
        account_id: Math.max(1, Number(draft?.account_id || 1)),
        account_label: accountLabel,
        provider,
        remote_draft_id: String(draft?.remote_draft_id || `draft-${provider}-${nextMailDraftCounter('remote_id')}`).trim(),
        reply_to_message_id: String(draft?.reply_to_message_id || '').trim(),
        thread_id: String(draft?.thread_id || '').trim(),
        status,
        to,
        cc,
        bcc,
        subject,
        body: String(draft?.body || ''),
        title: mailDraftTitle(subject),
        ref_path: String(draft?.ref_path || `.slopshell/artifacts/mail/draft-${artifactID}.md`).trim(),
      };
      normalized.meta = {
        account_id: normalized.account_id,
        account_label: normalized.account_label,
        provider: normalized.provider,
        remote_draft_id: normalized.remote_draft_id,
        reply_to_message_id: normalized.reply_to_message_id,
        thread_id: normalized.thread_id,
        status: normalized.status,
        to: normalized.to.slice(),
        cc: normalized.cc.slice(),
        bcc: normalized.bcc.slice(),
        subject: normalized.subject,
      };
      normalized.artifact = {
        id: normalized.artifact_id,
        kind: 'email_draft',
        title: normalized.title,
        ref_path: normalized.ref_path,
        meta_json: JSON.stringify(normalized.meta),
      };
      return normalized;
    }
    function cloneMailDraft(draft) {
      return JSON.parse(JSON.stringify(normalizeMailDraft(draft)));
    }
    function existingItemSidebarRow(itemID) {
      return itemSidebarStateKeys()
        .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : [])
        .find((entry) => Number(entry?.id || 0) === Number(itemID)) || null;
    }
    function syncMailDraftSidebarItem(draft) {
      const normalized = normalizeMailDraft(draft);
      const existing = existingItemSidebarRow(normalized.item_id);
      const stateKey = normalized.status === 'sent' ? 'done' : 'inbox';
      const entry = {
        id: normalized.item_id,
        title: normalized.title,
        state: stateKey,
        sphere: existing?.sphere || normalizeHarnessSphere(runtimeState.active_sphere),
        artifact_id: normalized.artifact_id,
        source: normalized.provider,
        source_ref: normalized.remote_draft_id,
        artifact_title: normalized.title,
        artifact_kind: 'email_draft',
        actor_name: '',
        created_at: String(existing?.created_at || '2026-03-10 11:00:00'),
        updated_at: '2026-03-10 11:05:00',
      };
      if (existing) {
        if (String(existing.state || '').trim().toLowerCase() !== stateKey) {
          moveItemSidebarEntry(normalized.item_id, stateKey, entry);
        } else {
          patchItemSidebarEntry(normalized.item_id, entry);
        }
      } else {
        prependItemSidebarEntry(entry, stateKey);
      }
      const artifacts = window.__itemSidebarArtifacts && typeof window.__itemSidebarArtifacts === 'object'
        ? window.__itemSidebarArtifacts
        : {};
      artifacts[String(normalized.artifact_id)] = { ...normalized.artifact };
      window.__itemSidebarArtifacts = artifacts;
      return normalized;
    }
    function providerAccountLabel(provider) {
      if (provider === 'exchange') return 'Work Exchange';
      if (provider === 'imap') return 'IMAP Inbox';
      return 'Private Gmail';
    }
    function findItemArtifact(item) {
      const artifacts = window.__itemSidebarArtifacts && typeof window.__itemSidebarArtifacts === 'object'
        ? window.__itemSidebarArtifacts
        : {};
      return artifacts[String(Number(item?.artifact_id || 0))] || artifacts[Number(item?.artifact_id || 0)] || null;
    }
    function parseArtifactMetaJSON(artifact) {
      try {
        return JSON.parse(String(artifact?.meta_json || '{}'));
      } catch (_) {
        return {};
      }
    }
    function createHarnessDraftFromRequest(body = {}) {
      const draft = syncMailDraftSidebarItem({
        artifact_id: nextMailDraftCounter('artifact_id'),
        item_id: nextMailDraftCounter('item_id'),
        account_id: Math.max(1, Number(body.account_id || 1)),
        account_label: providerAccountLabel(String(body.provider || 'gmail').trim().toLowerCase() || 'gmail'),
        provider: String(body.provider || 'gmail').trim().toLowerCase() || 'gmail',
        to: normalizeMailDraftAddresses(body.to),
        cc: normalizeMailDraftAddresses(body.cc),
        bcc: normalizeMailDraftAddresses(body.bcc),
        subject: String(body.subject || '').trim(),
        body: String(body.body || ''),
        status: 'draft',
      });
      const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
      drafts[String(draft.artifact_id)] = cloneMailDraft(draft);
      window.__mailDrafts = drafts;
      return cloneMailDraft(draft);
    }
    function createHarnessForwardDraft(itemID) {
      const rows = ['inbox', 'waiting', 'someday', 'done']
        .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
      const item = rows.find((entry) => Number(entry?.id || 0) === Number(itemID)) || null;
      if (!item) return null;
      const provider = String(item?.source || 'gmail').trim().toLowerCase() || 'gmail';
      const artifact = findItemArtifact(item);
      const meta = parseArtifactMetaJSON(artifact);
      const subjectSource = String(meta.subject || item?.artifact_title || item?.title || '').trim();
      const subject = /^fwd?:/i.test(subjectSource) ? subjectSource : `Fwd: ${subjectSource}`;
      const lastMessage = Array.isArray(meta.messages) && meta.messages.length > 0
        ? meta.messages[meta.messages.length - 1] : null;
      const fwdSender = lastMessage ? String(lastMessage.sender || '').trim() : String(meta.sender || '').trim();
      const fwdDate = lastMessage ? String(lastMessage.date || '').trim() : String(meta.date || '').trim();
      const fwdBody = lastMessage ? String(lastMessage.body || lastMessage.snippet || '').trim() : String(meta.body || meta.snippet || '').trim();
      const body = '---------- Forwarded message ----------'
        + (fwdSender ? '\nFrom: ' + fwdSender : '')
        + (fwdDate ? '\nDate: ' + fwdDate : '')
        + (subjectSource ? '\nSubject: ' + subjectSource : '')
        + '\n\n' + fwdBody;
      const draft = syncMailDraftSidebarItem({
        artifact_id: nextMailDraftCounter('artifact_id'),
        item_id: nextMailDraftCounter('item_id'),
        account_id: 1,
        account_label: providerAccountLabel(provider),
        provider,
        remote_draft_id: `draft-${provider}-${nextMailDraftCounter('remote_id')}`,
        thread_id: String(meta.thread_id || '').trim(),
        to: [],
        cc: [],
        bcc: [],
        subject,
        body,
        status: 'draft',
      });
      const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
      drafts[String(draft.artifact_id)] = cloneMailDraft(draft);
      window.__mailDrafts = drafts;
      return cloneMailDraft(draft);
    }
    function createHarnessReplyDraft(itemID) {
      const rows = ['inbox', 'waiting', 'someday', 'done']
        .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
      const item = rows.find((entry) => Number(entry?.id || 0) === Number(itemID)) || null;
      if (!item) return null;
      const provider = String(item?.source || 'gmail').trim().toLowerCase() || 'gmail';
      const artifact = findItemArtifact(item);
      const meta = parseArtifactMetaJSON(artifact);
      const sender = firstMailAddress(meta.sender || meta.from || (Array.isArray(meta.participants) ? meta.participants[0] : ''));
      const subjectSource = String(meta.subject || item?.artifact_title || item?.title || '').trim();
      const subject = /^re:/i.test(subjectSource) ? subjectSource : `Re: ${subjectSource}`;
      const draft = syncMailDraftSidebarItem({
        artifact_id: nextMailDraftCounter('artifact_id'),
        item_id: nextMailDraftCounter('item_id'),
        account_id: 1,
        account_label: providerAccountLabel(provider),
        provider,
        remote_draft_id: `draft-${provider}-${nextMailDraftCounter('remote_id')}`,
        reply_to_message_id: String(item?.source_ref || '').trim(),
        thread_id: String(meta.thread_id || '').trim(),
        to: sender ? [sender] : [],
        cc: [],
        bcc: [],
        subject,
        body: '',
        status: 'draft',
      });
      const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
      drafts[String(draft.artifact_id)] = cloneMailDraft(draft);
      window.__mailDrafts = drafts;
      return cloneMailDraft(draft);
    }
    function createHarnessReplyAllDraft(itemID) {
      const rows = ['inbox', 'waiting', 'someday', 'done']
        .flatMap((stateKey) => Array.isArray((window.__itemSidebarData || {})[stateKey]) ? (window.__itemSidebarData || {})[stateKey] : []);
      const item = rows.find((entry) => Number(entry?.id || 0) === Number(itemID)) || null;
      if (!item) return null;
      const provider = String(item?.source || 'gmail').trim().toLowerCase() || 'gmail';
      const artifact = findItemArtifact(item);
      const meta = parseArtifactMetaJSON(artifact);
      const sender = firstMailAddress(meta.sender || meta.from || (Array.isArray(meta.participants) ? meta.participants[0] : ''));
      const subjectSource = String(meta.subject || item?.artifact_title || item?.title || '').trim();
      const subject = /^re:/i.test(subjectSource) ? subjectSource : `Re: ${subjectSource}`;
      const recipients = Array.isArray(meta.recipients) ? meta.recipients : [];
      const senderLower = String(sender || '').trim().toLowerCase();
      const cc = recipients.filter((r) => {
        const lower = String(r || '').trim().toLowerCase();
        return lower && lower !== senderLower;
      });
      const draft = syncMailDraftSidebarItem({
        artifact_id: nextMailDraftCounter('artifact_id'),
        item_id: nextMailDraftCounter('item_id'),
        account_id: 1,
        account_label: providerAccountLabel(provider),
        provider,
        remote_draft_id: `draft-${provider}-${nextMailDraftCounter('remote_id')}`,
        reply_to_message_id: String(item?.source_ref || '').trim(),
        thread_id: String(meta.thread_id || '').trim(),
        to: sender ? [sender] : [],
        cc,
        bcc: [],
        subject,
        body: '',
        status: 'draft',
      });
      const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
      drafts[String(draft.artifact_id)] = cloneMailDraft(draft);
      window.__mailDrafts = drafts;
      return cloneMailDraft(draft);
    }
    function updateHarnessMailDraft(artifactID, patch = {}) {
      const drafts = window.__mailDrafts && typeof window.__mailDrafts === 'object' ? window.__mailDrafts : {};
      const current = drafts[String(artifactID)] || drafts[Number(artifactID)];
      if (!current) return null;
      const next = syncMailDraftSidebarItem({
        ...current,
        ...patch,
        artifact_id: current.artifact_id,
        item_id: current.item_id,
        status: patch.status || current.status || 'draft',
      });
      drafts[String(artifactID)] = cloneMailDraft(next);
      window.__mailDrafts = drafts;
      return cloneMailDraft(next);
    }
    window.__participantConfig = {
      companion_enabled: false,
      language: 'en',
      max_segment_duration_ms: 30000,
      session_ram_cap_mb: 64,
      stt_model: 'whisper-1',
      idle_surface: 'robot',
      audio_persistence: 'none',
      capture_source: 'microphone',
    };
    window.__companionRuntimeState = {
      state: 'idle',
      reason: 'idle',
      workspace_path: '/tmp/test',
      updated_at: Math.floor(Date.now() / 1000),
    };
    window.__setProjectRunStates = (states) => {
      const next = states && typeof states === 'object' ? states : {};
      harnessProjectRunStates = {
        ...harnessProjectRunStates,
        ...next,
      };
      void window._slopshellApp?.fetchProjects?.();
    };
    window.__setProjectActivity = (states) => {
      const next = states && typeof states === 'object' ? states : {};
      for (const [projectId, patch] of Object.entries(next)) {
        const index = harnessProjects.findIndex((project) => project.id === projectId);
        if (index < 0 || !patch || typeof patch !== 'object') continue;
        harnessProjects[index] = {
          ...harnessProjects[index],
          ...patch,
          unread: Boolean(patch.unread ?? harnessProjects[index].unread),
          review_pending: Boolean(patch.review_pending ?? harnessProjects[index].review_pending),
          chat_mode: String(patch.chat_mode || harnessProjects[index].chat_mode || 'chat'),
        };
      }
      void window._slopshellApp?.fetchProjects?.();
    };
    const cloneProject = (id) => {
      const project = harnessProjects.find((item) => item.id === id) || harnessProjects[0];
      return {
        ...project,
        sphere: String(project?.sphere || '').trim().toLowerCase(),
        run_state: { ...(harnessProjectRunStates[project.id] || project.run_state || {}) },
      };
    };
    function requestedSphere(url) {
      try {
        const parsed = new URL(url, window.location.origin);
        const sphere = String(parsed.searchParams.get('sphere') || '').trim().toLowerCase();
        if (sphere === 'work' || sphere === 'private') return sphere;
      } catch (_) {}
      return '';
    }
    function requestedItemFilters(url) {
      try {
        const parsed = new URL(url, window.location.origin);
        const source = String(parsed.searchParams.get('source') || '').trim().toLowerCase();
        const workspaceRaw = String(parsed.searchParams.get('workspace_id') || '').trim().toLowerCase();
        const contextID = Number(parsed.searchParams.get('context_id') || 0);
        const projectItemID = Number(parsed.searchParams.get('project_item_id') || 0);
        const actorID = Number(parsed.searchParams.get('actor_id') || 0);
        const section = String(parsed.searchParams.get('section') || '').trim().toLowerCase();
        return {
          source,
          workspace_id: workspaceRaw,
          context_id: Number.isFinite(contextID) && contextID > 0 ? contextID : 0,
          project_item_id: Number.isFinite(projectItemID) && projectItemID > 0 ? projectItemID : 0,
          actor_id: Number.isFinite(actorID) && actorID > 0 ? actorID : 0,
          section,
        };
      } catch (_) {
        return { source: '', workspace_id: '', context_id: 0, project_item_id: 0, actor_id: 0, section: '' };
      }
    }
    function descendantContextIDs(contextID) {
      const cleanID = Number(contextID || 0);
      if (!(cleanID > 0)) return [];
      const contexts = Array.isArray(window.__itemSidebarContexts) ? window.__itemSidebarContexts : defaultItemSidebarContexts();
      const childrenByParent = contexts.reduce((acc, entry) => {
        const parentID = Number(entry?.parent_id || 0);
        if (!acc.has(parentID)) acc.set(parentID, []);
        acc.get(parentID).push(Number(entry?.id || 0));
        return acc;
      }, new Map());
      const out = [];
      const queue = [cleanID];
      const seen = new Set();
      while (queue.length > 0) {
        const nextID = Number(queue.shift() || 0);
        if (!(nextID > 0) || seen.has(nextID)) continue;
        seen.add(nextID);
        out.push(nextID);
        const children = childrenByParent.get(nextID) || [];
        children.forEach((childID) => queue.push(childID));
      }
      return out;
    }
    function matchesItemFilters(item, sphere, filters) {
      const itemSphere = String(item?.sphere || '').trim().toLowerCase();
      if (sphere && itemSphere !== sphere) return false;
      const itemSource = String(item?.source || '').trim().toLowerCase();
      if (filters?.source && itemSource !== filters.source) return false;
      if (Number(filters?.actor_id || 0) > 0 && Number(item?.actor_id || 0) !== Number(filters.actor_id)) return false;
      if (Number(filters?.project_item_id || 0) > 0 && Number(item?.project_item_id || 0) !== Number(filters.project_item_id)) return false;
      if (filters?.section === 'project_items') {
        if (String(item?.kind || '').trim().toLowerCase() !== 'project') return false;
        if (String(item?.state || '').trim().toLowerCase() === 'done') return false;
      }
      if (filters?.workspace_id === 'null') {
        return item?.workspace_id == null;
      }
      if (Number.isFinite(Number(filters?.workspace_id)) && Number(filters.workspace_id) > 0) {
        return Number(item?.workspace_id || 0) === Number(filters.workspace_id);
      }
      if (filters?.workspace_id) {
        return String(item?.workspace_id || '').trim() === filters.workspace_id;
      }
      if (Number(filters?.context_id || 0) > 0) {
        const allowed = new Set(descendantContextIDs(filters.context_id));
        const itemContextIDs = Array.isArray(item?.context_ids) ? item.context_ids.map((value) => Number(value || 0)) : [];
        return itemContextIDs.some((value) => allowed.has(value));
      }
      return true;
    }
    function allItemSidebarEntries() {
      const itemData = window.__itemSidebarData || defaultItemSidebarData();
      return itemSidebarStateKeys().flatMap((key) => cloneItemSidebarEntries(itemData[key]));
    }
    function projectChildCounts(projectID) {
      const counts = {
        inbox: 0,
        next: 0,
        waiting: 0,
        deferred: 0,
        someday: 0,
        review: 0,
        done: 0,
        total: 0,
      };
      allItemSidebarEntries().forEach((item) => {
        if (Number(item?.project_item_id || 0) !== Number(projectID)) return;
        const stateKey = String(item?.state || '').trim().toLowerCase();
        if (Object.prototype.hasOwnProperty.call(counts, stateKey)) {
          counts[stateKey] += 1;
        }
        counts.total += 1;
      });
      return counts;
    }
    function projectItemReviewRows(sphere, filters) {
      const projectFilters = { ...(filters || {}), project_item_id: 0, section: 'project_items' };
      return allItemSidebarEntries()
        .filter((item) => matchesItemFilters(item, sphere, projectFilters))
        .map((item) => {
          const children = projectChildCounts(item.id);
          return {
            item,
            health: {
              has_next_action: children.next > 0,
              has_waiting: children.waiting > 0,
              has_deferred: children.deferred > 0,
              has_someday: children.someday > 0,
              stalled: children.next + children.waiting + children.deferred + children.someday === 0,
            },
            children,
          };
        });
    }
    function actorByID(actorID) {
      const actors = Array.isArray(window.__itemSidebarActors) ? window.__itemSidebarActors : defaultItemSidebarActors();
      return actors.find((actor) => Number(actor?.id || 0) === Number(actorID || 0)) || null;
    }
    function personDashboardMeta(actor) {
      let meta = {};
      try { meta = JSON.parse(String(actor?.meta_json || '{}')); } catch (_) { meta = {}; }
      const diagnostics = Array.isArray(meta?.diagnostics)
        ? meta.diagnostics.map((entry) => String(entry || '').trim()).filter(Boolean)
        : [];
      if (!String(meta?.person_path || '').trim() && !diagnostics.some((entry) => entry.startsWith('needs_person_note:'))) {
        diagnostics.push(`needs_person_note: ${String(actor?.name || '').trim()}`);
      }
      return {
        person_path: String(meta?.person_path || '').trim(),
        diagnostics,
      };
    }
    function personDashboardForActor(actorID, sphere, filters) {
      const actor = actorByID(actorID);
      if (!actor) return null;
      const scopedFilters = { ...(filters || {}), actor_id: Number(actorID || 0), section: '' };
      const items = allItemSidebarEntries().filter((item) => {
        if (String(item?.kind || 'action').trim().toLowerCase() === 'project') return false;
        return matchesItemFilters(item, sphere, scopedFilters);
      });
      const waiting = items.filter((item) => ['waiting', 'deferred'].includes(String(item?.state || '').trim().toLowerCase()));
      const owed = items.filter((item) => ['next', 'inbox'].includes(String(item?.state || '').trim().toLowerCase()));
      const closed = items.filter((item) => String(item?.state || '').trim().toLowerCase() === 'done');
      const childIDs = new Set(items.map((item) => Number(item?.id || 0)));
      const projectItems = allItemSidebarEntries().filter((item) => {
        if (String(item?.kind || '').trim().toLowerCase() !== 'project') return false;
        return allItemSidebarEntries().some((child) => Number(child?.project_item_id || 0) === Number(item?.id || 0) && childIDs.has(Number(child?.id || 0)));
      });
      const meta = personDashboardMeta(actor);
      return {
        actor,
        person: String(actor?.name || '').trim(),
        person_path: meta.person_path || undefined,
        counts: {
          waiting_on_them: waiting.length,
          i_owe_them: owed.length,
          recently_closed: closed.length,
          open: waiting.length + owed.length,
        },
        waiting_on_them: waiting,
        i_owe_them: owed,
        recently_closed: closed,
        project_items: projectItems,
        diagnostics: meta.diagnostics,
      };
    }
    function personDashboardRows(sphere, filters) {
      const actorIDs = new Set();
      allItemSidebarEntries().forEach((item) => {
        const actorID = Number(item?.actor_id || 0);
        if (!(actorID > 0)) return;
        const actor = actorByID(actorID);
        if (String(actor?.kind || '').trim().toLowerCase() !== 'human') return;
        if (String(item?.kind || 'action').trim().toLowerCase() === 'project') return;
        if (matchesItemFilters(item, sphere, { ...(filters || {}), actor_id: 0, section: '' })) {
          actorIDs.add(actorID);
        }
      });
      return Array.from(actorIDs)
        .map((actorID) => personDashboardForActor(actorID, sphere, filters))
        .filter((row) => row && Number(row.counts?.open || 0) > 0)
        .sort((left, right) => {
          if (left.counts.open !== right.counts.open) return right.counts.open - left.counts.open;
          return String(left.person || '').localeCompare(String(right.person || ''));
        });
    }
    const nextHarnessProjectId = (kind) => {
      const cleanKind = String(kind || 'project').trim().toLowerCase() || 'project';
      let suffix = 1;
      while (harnessProjects.some((project) => project.id === `${cleanKind}-${suffix}`)) {
        suffix += 1;
      }
      return `${cleanKind}-${suffix}`;
    };
