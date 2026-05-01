// Slopshell Playwright harness — fetch routes for setup, runtime preferences,
// workspaces, files, transcripts, summaries, meetings, references, markdown,
// companion config, and runtime/preferences.
__harnessRouteHandlers.push(async function harnessRouteRuntime(u, opts) {
      if (u.includes('/api/setup')) {
        return new Response(JSON.stringify({
          has_password: true,
          authenticated: true,
          local_session: 'local',
        }), { status: 200 });
      }
      if (u.includes('/api/artifacts/taxonomy')) {
        return new Response(JSON.stringify(artifactTaxonomy), { status: 200 });
      }
      if (u.includes('/api/runtime/preferences') && opts?.method === 'PATCH') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        if (Object.prototype.hasOwnProperty.call(body, 'silent_mode')) {
          runtimeState.silent_mode = Boolean(body.silent_mode);
        }
        if (Object.prototype.hasOwnProperty.call(body, 'fast_mode')) {
          runtimeState.fast_mode = Boolean(body.fast_mode);
        }
        if (typeof body?.tool === 'string') {
          const normalized = String(body.tool).trim().toLowerCase();
          runtimeState.tool = ['pointer', 'highlight', 'ink', 'text_note', 'prompt'].includes(normalized)
            ? normalized
            : 'pointer';
        }
        if (typeof body?.startup_behavior === 'string') {
          runtimeState.startup_behavior = 'resume_active';
        }
        if (typeof body?.active_sphere === 'string') {
          runtimeState.active_sphere = String(body.active_sphere).trim().toLowerCase() === 'work' ? 'work' : 'private';
        }
        if (typeof body?.turn_policy_profile === 'string') {
          const normalized = String(body.turn_policy_profile).trim().toLowerCase();
          runtimeState.turn_policy_profile = ['balanced', 'patient', 'assertive'].includes(normalized)
            ? normalized
            : 'balanced';
        }
        if (Object.prototype.hasOwnProperty.call(body, 'turn_eval_logging_enabled')) {
          runtimeState.turn_eval_logging_enabled = Boolean(body.turn_eval_logging_enabled);
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'runtime_preferences',
          method: opts?.method || 'PATCH',
          url: u,
          payload: { ...runtimeState },
        });
        return new Response(JSON.stringify({
          ok: true,
          silent_mode: runtimeState.silent_mode,
          fast_mode: runtimeState.fast_mode,
          live_policy: runtimeState.live_policy,
          tool: runtimeState.tool,
          startup_behavior: runtimeState.startup_behavior,
          active_sphere: runtimeState.active_sphere,
          turn_policy_profile: runtimeState.turn_policy_profile,
          turn_eval_logging_enabled: runtimeState.turn_eval_logging_enabled,
        }), { status: 200 });
      }
      if (u.includes('/api/live-policy') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        runtimeState.live_policy = String(body.policy || '').trim().toLowerCase() === 'meeting' ? 'meeting' : 'dialogue';
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'live_policy',
          method: opts?.method || 'POST',
          url: u,
          payload: { policy: runtimeState.live_policy },
        });
        return new Response(JSON.stringify({ policy: runtimeState.live_policy }), { status: 200 });
      }
      if (u.includes('/api/live-policy')) {
        return new Response(JSON.stringify({ policy: runtimeState.live_policy }), { status: 200 });
      }
      if (u.includes('/api/dialogue/diagnostics') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'dialogue_diagnostics',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      if (u.includes('/api/workspace/focus')) {
        const payload = normalizeHarnessWorkspaceFocus(harnessWorkspaceFocus);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'workspace_focus',
          method: opts?.method || 'GET',
          url: u,
          payload,
        });
        return new Response(JSON.stringify(payload), { status: 200 });
      }
      if (u.includes('/api/workspaces/busy')) {
        const states = harnessWorkspaceBusyStates.map((entry, index) => normalizeHarnessWorkspaceBusyState(entry, index));
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'workspace_busy',
          method: opts?.method || 'GET',
          url: u,
          payload: { count: states.length },
        });
        return new Response(JSON.stringify({ ok: true, states }), { status: 200 });
      }
      if (u.includes('/api/bugs/report') && opts?.method === 'POST') {
        let body = {};
        try {
          body = JSON.parse(String(opts?.body || '{}'));
        } catch (_) {
          body = {};
        }
        const bugReportMode = String(window.__slopshellBugReportTestEnv?.issueMode || '').trim().toLowerCase();
        window.__bugReportRequests.push(body);
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'bug_report',
          method: opts?.method || 'POST',
          url: u,
          trigger: body.trigger || '',
        });
        if (bugReportMode !== 'local') {
          prependInboxItem({
            id: 103,
            title: 'Bug report: Harness repro',
            state: 'inbox',
            sphere: 'private',
            artifact_id: 0,
            source: 'github',
            source_ref: 'sloppy-org/slopshell#77',
            artifact_title: 'Bug report: Harness repro',
            artifact_kind: 'github_issue',
            actor_name: '',
            created_at: '2026-03-08 15:04:05',
            updated_at: '2026-03-08 15:04:05',
          });
        }
        const payload = {
          ok: true,
          bundle_path: '.slopshell/artifacts/bugs/20260308-150405-abcd1234/bundle.json',
          screenshot_path: '.slopshell/artifacts/bugs/20260308-150405-abcd1234/screenshot.png',
          annotated_path: body.annotated_data_url ? '.slopshell/artifacts/bugs/20260308-150405-abcd1234/annotated.png' : '',
          issue_title: 'Bug report: Harness repro',
        };
        if (bugReportMode === 'local') {
          payload.issue_error = 'auto-filing skipped: add a short note or capture clearer interaction context';
          return new Response(JSON.stringify(payload), { status: 200 });
        }
        payload.issue_number = 77;
        payload.issue_url = 'https://github.com/sloppy-org/slopshell/issues/77';
        payload.item_id = 501;
        return new Response(JSON.stringify(payload), { status: 200 });
      }
      if (u.endsWith('/api/runtime/workspaces') && opts?.method === 'POST') {
        let body = {};
        try {
          body = JSON.parse(String(opts?.body || '{}'));
        } catch (_) {
          body = {};
        }
        const kind = String(body?.kind || 'managed').trim().toLowerCase();
        const id = nextHarnessProjectId(kind);
        const label = kind === 'meeting' ? 'Meeting' : (kind === 'task' ? 'Task' : 'Project');
        const project = {
          id,
          name: `${label} ${id.split('-').pop()}`,
          kind,
          workspace_path: `/tmp/${id}`,
          root_path: `/tmp/${id}`,
          source_workspace_id: String(body?.source_workspace_id || '').trim(),
          source_path: String(body?.source_path || '').trim(),
          chat_session_id: `chat-${id}`,
          canvas_session_id: id,
          chat_mode: 'chat',
          chat_model: 'local',
          chat_model_reasoning_effort: 'none',
          unread: false,
          review_pending: false,
          run_state: { active_turns: 0, queued_turns: 0, is_working: false, status: 'idle' },
        };
        harnessProjects.push(project);
        harnessProjectRunStates[id] = { ...project.run_state };
        if (Boolean(body?.activate)) {
          activeWorkspaceId = id;
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_create',
          method: opts?.method || 'POST',
          url: u,
          payload: {
            kind,
            path: String(body?.path || '').trim(),
            workspace_id: id,
            source_workspace_id: String(body?.source_workspace_id || '').trim(),
            source_path: String(body?.source_path || '').trim(),
          },
        });
        return new Response(JSON.stringify({
          ok: true,
          created: true,
          activated: Boolean(body?.activate),
          workspace: cloneProject(id),
        }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces') && u.includes('/activate')) {
        const activateId = decodeURIComponent(u.split('/api/runtime/workspaces/')[1].split('/activate')[0] || '').trim();
        const index = harnessProjects.findIndex((project) => project.id === activateId);
        if (index < 0) {
          return new Response('project not found', { status: 404 });
        }
        activeWorkspaceId = harnessProjects[index].id;
        if (!(harnessProjects[index].review_pending && harnessProjects[index].chat_mode === 'review')) {
          harnessProjects[index] = {
            ...harnessProjects[index],
            unread: false,
          };
        }
        const project = cloneProject(activateId);
        if (project.sphere === 'work' || project.sphere === 'private') {
          runtimeState.active_sphere = project.sphere;
        }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_activate',
          method: opts?.method || 'POST',
          url: u,
          payload: { workspace_id: project.id },
        });
        return new Response(JSON.stringify({
          ok: true,
          active_workspace_id: project.id,
          active_sphere: runtimeState.active_sphere,
          workspace: project,
        }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces/') && u.includes('/persist') && opts?.method === 'POST') {
        const projectId = decodeURIComponent(u.split('/api/runtime/workspaces/')[1].split('/persist')[0] || '').trim();
        const index = harnessProjects.findIndex((project) => project.id === projectId);
        if (index < 0) {
          return new Response('project not found', { status: 404 });
        }
        harnessProjects[index] = {
          ...harnessProjects[index],
          kind: 'managed',
        };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_persist',
          method: opts?.method || 'POST',
          url: u,
          payload: { workspace_id: projectId },
        });
        return new Response(JSON.stringify({
          ok: true,
          workspace: cloneProject(projectId),
        }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces/') && u.includes('/discard') && opts?.method === 'POST') {
        const projectId = decodeURIComponent(u.split('/api/runtime/workspaces/')[1].split('/discard')[0] || '').trim();
        const index = harnessProjects.findIndex((project) => project.id === projectId);
        if (index < 0) {
          return new Response('project not found', { status: 404 });
        }
        harnessProjects.splice(index, 1);
        delete harnessProjectRunStates[projectId];
        activeWorkspaceId = harnessProjects[0]?.id || '';
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_discard',
          method: opts?.method || 'POST',
          url: u,
          payload: { workspace_id: projectId, active_workspace_id: activeWorkspaceId },
        });
        return new Response(JSON.stringify({
          ok: true,
          discarded_project: projectId,
          active_workspace_id: activeWorkspaceId,
          workspace: cloneProject(activeWorkspaceId),
        }), { status: 200 });
      }
      if (u.includes('/api/hotword/status')) {
        return new Response(JSON.stringify({
          ...hotwordStatus,
        }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces') && u.includes('/chat-model') && opts?.method === 'POST') {
        const projectId = decodeURIComponent(u.split('/api/runtime/workspaces/')[1].split('/chat-model')[0] || '').trim();
        const index = harnessProjects.findIndex((project) => project.id === projectId);
        if (index < 0) {
          return new Response('project not found', { status: 404 });
        }
        let body = {};
        try {
          body = JSON.parse(String(opts?.body || '{}'));
        } catch (_) {
          body = {};
        }
        const model = String(body?.model || harnessProjects[index].chat_model || 'local').trim().toLowerCase();
        let effort = String(body?.reasoning_effort || harnessProjects[index].chat_model_reasoning_effort || 'none').trim().toLowerCase();
        if (effort === 'extra_high') effort = 'xhigh';
        harnessProjects[index] = {
          ...harnessProjects[index],
          chat_model: model || harnessProjects[index].chat_model,
          chat_model_reasoning_effort: effort || harnessProjects[index].chat_model_reasoning_effort,
        };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'project_chat_model',
          method: opts?.method || 'POST',
          url: u,
          payload: { workspace_id: projectId, model, reasoning_effort: effort },
        });
        return new Response(JSON.stringify({
          workspace: cloneProject(projectId),
        }), { status: 200 });
      }
      if (u.includes('/api/runtime/workspaces/activity')) {
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'projects_activity',
          method: opts?.method || 'GET',
          url: u,
        });
        const workspaces = harnessProjects.map((project) => ({
          workspace_id: project.id,
          workspace_path: project.workspace_path,
          name: project.name,
          kind: project.kind,
          chat_session_id: project.chat_session_id,
          chat_mode: project.chat_mode,
          unread: Boolean(project.unread),
          review_pending: Boolean(project.review_pending),
          run_state: { ...(harnessProjectRunStates[project.id] || project.run_state || {}) },
        }));
        return new Response(JSON.stringify({ ok: true, workspaces }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/files')) {
        const query = new URL(u, window.location.href).searchParams;
        const path = String(query.get('path') || '').trim();
        const cleanedPath = path.replace(/^\/+|\/+$/g, '');
        const workspaceSegment = decodeURIComponent(u.split('/api/workspaces/')[1].split('/files')[0] || '');
        const overrides = window.__mockWorkspaceFiles || null;
        const overrideKeys = [
          `${workspaceSegment}|${cleanedPath}`,
          `${workspaceSegment}|*`,
          `*|${cleanedPath}`,
        ];
        for (const key of overrideKeys) {
          if (overrides && Object.prototype.hasOwnProperty.call(overrides, key)) {
            const entries = overrides[key];
            return new Response(JSON.stringify({
              ok: true,
              workspace_id: workspaceSegment || activeWorkspaceId,
              path: cleanedPath,
              is_root: cleanedPath === '',
              entries: Array.isArray(entries) ? entries : [],
            }), { status: 200 });
          }
        }
        if (!cleanedPath) {
          return new Response(JSON.stringify({
            ok: true,
            workspace_id: activeWorkspaceId,
            path: '',
            is_root: true,
            entries: [
              { name: 'docs', path: 'docs', is_dir: true },
              { name: 'NOTES.md', path: 'NOTES.md', is_dir: false },
              { name: 'README.md', path: 'README.md', is_dir: false },
            ],
          }), { status: 200 });
        }
        if (cleanedPath === 'docs') {
          return new Response(JSON.stringify({
            ok: true,
            workspace_id: activeWorkspaceId,
            path: 'docs',
            is_root: false,
            entries: [
              { name: 'architecture.md', path: 'docs/architecture.md', is_dir: false },
              { name: 'guide.md', path: 'docs/guide.md', is_dir: false },
            ],
          }), { status: 200 });
        }
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          path: cleanedPath,
          is_root: false,
          entries: [],
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/transcript')) {
        const format = new URL(u, window.location.href).searchParams.get('format') || 'json';
        if (format === 'md') {
          return new Response('# Meeting Transcript\n\n- **Speaker** (10:00:00): Harness meeting transcript\n', {
            status: 200,
            headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
          });
        }
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          sessions: [{ id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' }],
          session: { id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' },
          segments: [{ id: 1, session_id: 'psess-harness-001', start_ts: 100, end_ts: 100, speaker: 'Speaker', text: 'Harness meeting transcript', model: 'whisper-1', latency_ms: 10, committed_at: 100, status: 'final' }],
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/summary')) {
        const format = new URL(u, window.location.href).searchParams.get('format') || 'json';
        if (format === 'md') {
          return new Response('# Meeting Summary\n\nHarness meeting summary\n', {
            status: 200,
            headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
          });
        }
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          sessions: [{ id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' }],
          session: { id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' },
          summary_text: 'Harness meeting summary',
          updated_at: 101,
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/meeting/finalize') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'meeting_finalize',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          session_id: 'psess-harness-001',
          summary_text: '# Meeting Notes\n\n## Decisions\n\n- Harness decision\n',
          summary_artifact_id: 701,
          transcript_discarded: Boolean(body.discard_transcript),
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/meeting-items') && opts?.method === 'POST') {
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        const proposals = Array.isArray(window.__meetingSummaryProposals) ? window.__meetingSummaryProposals : defaultMeetingSummaryProposals();
        const selected = Array.isArray(body.selected) ? body.selected : [];
        const nextIDBase = 900 + (Array.isArray((window.__itemSidebarData || {}).inbox) ? window.__itemSidebarData.inbox.length : 0);
        const createdItems = selected
          .map((index) => Number(index))
          .filter((index, position, all) => Number.isInteger(index) && index >= 0 && all.indexOf(index) === position)
          .map((index) => proposals.find((entry) => Number(entry?.index ?? -1) === index))
          .filter(Boolean)
          .map((proposal, offset) => prependInboxItem({
            id: nextIDBase + offset,
            title: String(proposal.title || ''),
            state: 'inbox',
            artifact_id: 701,
            source: 'meeting_summary',
            source_ref: `psess-harness-001:${Number(proposal.index || 0)}`,
            artifact_title: 'Meeting Summary',
            artifact_kind: 'markdown',
            actor_name: String(proposal.actor_name || ''),
            created_at: '2026-03-08 15:00:00',
            updated_at: '2026-03-08 15:00:00',
          }));
        window.__itemSidebarArtifacts = {
          ...(window.__itemSidebarArtifacts || {}),
          701: {
            id: 701,
            kind: 'markdown',
            title: 'Meeting Summary',
            meta_json: JSON.stringify({ summary: 'Harness meeting summary' }),
          },
        };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'meeting_items_create',
          method: opts?.method || 'POST',
          url: u,
          payload: body,
        });
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          session: { id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' },
          proposed_items: proposals,
          created_items: createdItems,
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/meeting-items')) {
        const proposals = Array.isArray(window.__meetingSummaryProposals) ? window.__meetingSummaryProposals : defaultMeetingSummaryProposals();
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          sessions: [{ id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' }],
          session: { id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' },
          summary_text: 'Harness meeting summary',
          proposed_items: proposals,
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/references')) {
        const format = new URL(u, window.location.href).searchParams.get('format') || 'json';
        if (format === 'md') {
          return new Response('# Meeting References\n\n## Entities\n\n- Acme\n\n## Topic Timeline\n\n- Budget\n', {
            status: 200,
            headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
          });
        }
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: '/tmp/test',
          sessions: [{ id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' }],
          session: { id: 'psess-harness-001', workspace_path: '/tmp/test', started_at: 100, ended_at: 0, config_json: '{}' },
          entities: ['Acme'],
          topic_timeline: ['Budget'],
        }), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/markdown-link/resolve')) {
        const payload = window.__mockMarkdownLinkResolution || {
          ok: false,
          blocked: true,
          reason: 'link blocked',
        };
        return new Response(JSON.stringify(payload), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (u.includes('/api/workspaces/') && u.includes('/markdown-link/panel')) {
        const payload = window.__mockMarkdownLinkPanel || {
          ok: true,
          source_path: 'topics/active.md',
          outgoing: [],
          broken_count: 0,
          backlinks: [],
        };
        return new Response(JSON.stringify(payload), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (u.includes('/api/workspaces/') && u.includes('/markdown-link/file')) {
        return new Response(String(window.__mockMarkdownLinkFileText || ''), {
          status: 200,
          headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
        });
      }
      if (u.includes('/api/workspaces/') && u.includes('/graph')) {
        const payload = window.__mockWorkspaceLocalGraph || {
          ok: true,
          source_path: 'topics/active.md',
          nodes: [],
          edges: [],
        };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'local_graph',
          method: opts?.method || 'GET',
          url: u,
        });
        return new Response(JSON.stringify(payload), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (u.includes('/api/workspaces/') && u.includes('/brain-canvas/cards/') && u.includes('/open')) {
        const cardID = String(u.split('/cards/')[1] || '').split('/')[0];
        const card = (window.__mockBrainCanvas?.cards || []).find((entry) => String(entry?.id || '') === cardID);
        const payload = card
          ? {
            ok: !card.stale,
            kind: card.binding?.kind || 'unknown',
            open_url: card.open_url || '',
            title: card.title || '',
            body: card.body || '',
            binding: card.binding || { kind: 'unknown' },
            error: card.stale ? card.reason || 'stale' : '',
          }
          : { ok: false, kind: 'unknown', binding: { kind: 'unknown' }, error: 'card not found' };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'brain_canvas_open',
          method: opts?.method || 'GET',
          url: u,
          card_id: cardID,
        });
        return new Response(JSON.stringify(payload), { status: card ? 200 : 404, headers: { 'Content-Type': 'application/json' } });
      }
      if (u.includes('/api/workspaces/') && u.includes('/brain-canvas/cards/') && opts?.method === 'PATCH') {
        const cardID = String(u.split('/cards/')[1] || '').split('/')[0];
        let body = {};
        try { body = JSON.parse(String(opts?.body || '{}')); } catch (_) { body = {}; }
        window.__brainCanvasPatchLog = window.__brainCanvasPatchLog || [];
        window.__brainCanvasPatchLog.push({ card_id: cardID, payload: body });
        const cards = (window.__mockBrainCanvas?.cards || []).map((entry) => {
          if (String(entry?.id || '') !== cardID) return entry;
          const next = { ...entry };
          if (Object.prototype.hasOwnProperty.call(body, 'x')) next.x = Number(body.x) || 0;
          if (Object.prototype.hasOwnProperty.call(body, 'y')) next.y = Number(body.y) || 0;
          if (Object.prototype.hasOwnProperty.call(body, 'width')) next.width = Number(body.width) || entry.width;
          if (Object.prototype.hasOwnProperty.call(body, 'height')) next.height = Number(body.height) || entry.height;
          if (Object.prototype.hasOwnProperty.call(body, 'title')) next.title = String(body.title || '');
          if (Object.prototype.hasOwnProperty.call(body, 'body')) next.body = String(body.body || '');
          return next;
        });
        if (window.__mockBrainCanvas) window.__mockBrainCanvas.cards = cards;
        const updated = cards.find((entry) => String(entry?.id || '') === cardID) || { id: cardID };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'brain_canvas_patch',
          method: 'PATCH',
          url: u,
          card_id: cardID,
          payload: body,
        });
        return new Response(JSON.stringify(updated), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (u.includes('/api/workspaces/') && u.includes('/brain-canvas')) {
        const payload = window.__mockBrainCanvas || { ok: true, name: 'default', cards: [] };
        window.__harnessLog.push({
          type: 'api_fetch',
          action: 'brain_canvas_load',
          method: opts?.method || 'GET',
          url: u,
        });
        return new Response(JSON.stringify(payload), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (u.includes('/api/workspaces/') && u.includes('/companion/config') && opts?.method === 'PUT') {
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
      if (u.includes('/api/workspaces/') && u.includes('/companion/config')) {
        return new Response(JSON.stringify(window.__participantConfig), { status: 200 });
      }
      if (u.includes('/api/workspaces/') && u.includes('/companion/state')) {
        const runtime = window.__companionRuntimeState || {};
        return new Response(JSON.stringify({
          ok: true,
          workspace_id: activeWorkspaceId,
          workspace_path: String(runtime.workspace_path || '/tmp/test'),
          state: String(runtime.state || 'idle'),
          runtime: {
            state: String(runtime.state || 'idle'),
            reason: String(runtime.reason || 'idle'),
            workspace_path: String(runtime.workspace_path || '/tmp/test'),
            updated_at: Number(runtime.updated_at || Math.floor(Date.now() / 1000)),
          },
          companion_enabled: Boolean(window.__participantConfig.companion_enabled),
          idle_surface: String(window.__participantConfig.idle_surface || 'robot'),
          audio_persistence: 'none',
          capture_source: 'microphone',
          active_sessions: Boolean(window.__participantConfig.companion_enabled) ? 1 : 0,
          active_session_id: Boolean(window.__participantConfig.companion_enabled) ? 'psess-harness-001' : '',
          directed_speech_gate: { decision: 'disabled', reason: 'harness' },
          interaction_policy: { decision: 'disabled', reason: 'harness' },
          config: { ...window.__participantConfig },
        }), { status: 200 });
      }
    });
