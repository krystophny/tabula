// Slopshell Playwright harness — default mock fixtures (workspaces, artifacts, meetings).

    const harnessProjects = [
      {
        id: 'test',
        name: 'Test',
        kind: 'managed',
        sphere: 'private',
        workspace_path: '/tmp/test',
        root_path: '/tmp',
        chat_session_id: 'chat-1',
        canvas_session_id: 'local',
        chat_mode: 'chat',
        chat_model: 'local',
        chat_model_reasoning_effort: 'none',
        unread: false,
        review_pending: false,
        run_state: { active_turns: 0, queued_turns: 0, is_working: false, status: 'idle' },
      },
    ];
    let activeWorkspaceId = 'test';
    let harnessProjectRunStates = {
      test: { active_turns: 0, queued_turns: 0, is_working: false, status: 'idle' },
    };
    let harnessWorkspaceFocus = {
      anchor: {
        id: 1,
        name: 'Daily Workspace',
        dir_path: '/tmp/daily',
        sphere: 'private',
        is_daily: true,
      },
      focus: {
        id: 1,
        name: 'Daily Workspace',
        dir_path: '/tmp/daily',
        sphere: 'private',
        is_daily: true,
      },
      explicit: false,
    };
    let harnessWorkspaceBusyStates = [
      {
        workspace_id: 1,
        workspace_name: 'Daily Workspace',
        dir_path: '/tmp/daily',
        is_daily: true,
        is_anchor: true,
        is_focused: true,
        active_turns: 0,
        queued_turns: 0,
        status: 'idle',
      },
    ];
    function normalizeHarnessWorkspace(workspace, fallback = {}) {
      const source = workspace && typeof workspace === 'object' ? workspace : {};
      const fallbackSource = fallback && typeof fallback === 'object' ? fallback : {};
      const id = Math.max(0, Number(source.id ?? fallbackSource.id ?? 0) || 0);
      const dirPath = String(source.dir_path ?? fallbackSource.dir_path ?? '').trim();
      const name = String(source.name ?? fallbackSource.name ?? '').trim() || 'Workspace';
      const rawSphere = source.sphere ?? fallbackSource.sphere ?? runtimeState.active_sphere ?? 'private';
      return {
        id,
        name,
        dir_path: dirPath,
        sphere: String(rawSphere).trim().toLowerCase() === 'work' ? 'work' : 'private',
        is_daily: Boolean(source.is_daily ?? fallbackSource.is_daily),
      };
    }
    function normalizeHarnessWorkspaceFocus(snapshot) {
      const source = snapshot && typeof snapshot === 'object' ? snapshot : {};
      const anchor = normalizeHarnessWorkspace(source.anchor, harnessWorkspaceFocus.anchor);
      const focus = normalizeHarnessWorkspace(source.focus, anchor);
      return {
        anchor,
        focus,
        explicit: Boolean(source.explicit),
      };
    }
    function normalizeHarnessWorkspaceBusyState(state, index = 0) {
      const source = state && typeof state === 'object' ? state : {};
      const workspaceID = Math.max(0, Number(source.workspace_id || source.id || index + 1) || index + 1);
      const dirPath = String(source.dir_path || '').trim();
      const workspaceName = String(source.workspace_name || source.name || '').trim() || `Workspace ${workspaceID}`;
      const activeTurns = Math.max(0, Number(source.active_turns || 0) || 0);
      const queuedTurns = Math.max(0, Number(source.queued_turns || 0) || 0);
      let status = String(source.status || '').trim().toLowerCase();
      if (status !== 'running' && status !== 'queued' && status !== 'idle') {
        status = activeTurns > 0 ? 'running' : (queuedTurns > 0 ? 'queued' : 'idle');
      }
      return {
        workspace_id: workspaceID,
        workspace_name: workspaceName,
        dir_path: dirPath,
        is_daily: Boolean(source.is_daily),
        is_anchor: Boolean(source.is_anchor),
        is_focused: Boolean(source.is_focused),
        active_turns: activeTurns,
        queued_turns: queuedTurns,
        status,
      };
    }
    window.__setWorkspaceFocus = (snapshot) => {
      harnessWorkspaceFocus = normalizeHarnessWorkspaceFocus(snapshot);
    };
    window.__setWorkspaceBusyStates = (states) => {
      const source = Array.isArray(states) ? states : [];
      harnessWorkspaceBusyStates = source.map((entry, index) => normalizeHarnessWorkspaceBusyState(entry, index));
    };
    window.__setProjects = (projects, nextActiveProjectId = '') => {
      const source = Array.isArray(projects) ? projects : [];
      const normalized = source
        .map((project) => ({
          id: String(project?.id || '').trim(),
          name: String(project?.name || '').trim() || 'Project',
          kind: String(project?.kind || 'managed').trim().toLowerCase() || 'managed',
          sphere: String(project?.sphere || '').trim().toLowerCase(),
          workspace_path: String(project?.workspace_path || `/tmp/${String(project?.id || '').trim() || 'project'}`).trim(),
          root_path: String(project?.root_path || `/tmp/${String(project?.id || '').trim() || 'project'}`).trim(),
          source_workspace_id: String(project?.source_workspace_id || '').trim(),
          source_path: String(project?.source_path || '').trim(),
          chat_session_id: String(project?.chat_session_id || `chat-${String(project?.id || '').trim() || 'project'}`).trim(),
          canvas_session_id: String(project?.canvas_session_id || 'local').trim(),
          chat_mode: String(project?.chat_mode || 'chat').trim().toLowerCase() || 'chat',
          chat_model: String(project?.chat_model || 'local').trim().toLowerCase() || 'local',
          chat_model_reasoning_effort: String(project?.chat_model_reasoning_effort || 'none').trim().toLowerCase() || 'none',
          unread: Boolean(project?.unread),
          review_pending: Boolean(project?.review_pending),
          run_state: {
            active_turns: Number(project?.run_state?.active_turns || 0),
            queued_turns: Number(project?.run_state?.queued_turns || 0),
            is_working: Boolean(project?.run_state?.is_working),
            status: String(project?.run_state?.status || 'idle').trim().toLowerCase() || 'idle',
            active_turn_id: String(project?.run_state?.active_turn_id || '').trim(),
          },
        }))
        .filter((project) => project.id);
      harnessProjects.splice(0, harnessProjects.length, ...normalized);
      harnessProjectRunStates = normalized.reduce((acc, project) => {
        acc[project.id] = { ...project.run_state };
        return acc;
      }, {});
      const preferredActive = String(nextActiveProjectId || '').trim();
      if (preferredActive && normalized.some((project) => project.id === preferredActive)) {
        activeWorkspaceId = preferredActive;
      } else if (!normalized.some((project) => project.id === activeWorkspaceId)) {
        activeWorkspaceId = normalized[0]?.id || '';
      }
      void window._slopshellApp?.fetchProjects?.();
    };
    const defaultItemSidebarActors = () => ([
      { id: 1, name: 'Alice', kind: 'human', email: 'alice@example.com' },
      { id: 2, name: 'Bob', kind: 'human', email: 'bob@example.com' },
      { id: 3, name: 'Codex', kind: 'agent', email: 'codex@example.com' },
    ]);
    const defaultItemSidebarWorkspaces = () => ([
      { id: 1, name: 'Alpha', dir_path: '/tmp/alpha', sphere: 'private', is_active: true },
      { id: 2, name: 'Beta', dir_path: '/tmp/beta', sphere: 'work', is_active: false },
    ]);
    const defaultItemSidebarContexts = () => ([
      { id: 1, name: 'Work', parent_id: null },
      { id: 2, name: 'W7x', parent_id: 1 },
      { id: 3, name: 'Private', parent_id: null },
    ]);
    const defaultItemSidebarArtifacts = () => ({
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
          notes: ['Break parser cleanup into a small refactor, a test pass, and one cleanup issue.'],
        }),
      },
      502: {
        id: 502,
        kind: 'email',
        title: 'Re: triage follow-up',
        meta_json: JSON.stringify({
          subject: 'Re: triage follow-up',
          sender: 'Ada <ada@example.com>',
          recipients: ['team@example.com'],
          date: '2026-03-08T10:06:00Z',
          body: 'Need a response before tomorrow morning. Confirm whether the review packet is ready.',
        }),
      },
      505: {
        id: 505,
        kind: 'email_thread',
        title: 'Urgent follow-up',
        meta_json: JSON.stringify({
          subject: 'Urgent follow-up',
          message_count: 2,
          participants: ['Ada <ada@example.com>', 'Bob <bob@example.com>'],
          messages: [
            {
              sender: 'Ada <ada@example.com>',
              recipients: ['Bob <bob@example.com>'],
              date: '2026-03-08T10:04:00Z',
              body: 'Need a response before tomorrow morning.',
            },
            {
              sender: 'Bob <bob@example.com>',
              recipients: ['Ada <ada@example.com>'],
              date: '2026-03-08T10:05:00Z',
              body: 'I can confirm the review packet is ready.',
            },
          ],
        }),
      },
      503: {
        id: 503,
        kind: 'plan_note',
        title: 'Gesture backlog',
        meta_json: JSON.stringify({
          text: 'Sketch quick triage affordances for small touch screens.',
        }),
      },
      504: {
        id: 504,
        kind: 'github_issue',
        title: 'Capture checklist',
        meta_json: JSON.stringify({
          text: 'Close the remaining capture tasks before cutting the release.',
        }),
      },
    });
    const defaultItemSidebarData = () => ({
      inbox: [
        {
          id: 101,
          title: 'Review parser cleanup',
          state: 'inbox',
          sphere: 'private',
          context_ids: [3],
          artifact_id: 501,
          source: 'github',
          source_ref: 'owner/repo#177',
          artifact_title: 'Parser cleanup plan',
          artifact_kind: 'idea_note',
          actor_id: 1,
          actor_name: 'Alice',
          created_at: '2026-03-08 09:40:00',
          updated_at: '2026-03-08 09:58:00',
        },
        {
          id: 102,
          title: 'Answer triage email',
          state: 'inbox',
          sphere: 'private',
          context_ids: [3],
          artifact_id: 502,
          source: 'exchange',
          source_ref: 'msg-102',
          artifact_title: 'Re: triage follow-up',
          artifact_kind: 'email',
          actor_id: 2,
          actor_name: 'Bob',
          created_at: '2026-03-08 09:10:00',
          updated_at: '2026-03-08 09:12:00',
        },
      ],
      waiting: [
        {
          id: 201,
          title: 'Await review feedback',
          state: 'waiting',
          sphere: 'private',
          context_ids: [3],
          artifact_id: 0,
          source: 'github',
          source_ref: 'owner/repo#144',
          artifact_title: 'PR 144',
          artifact_kind: 'github_pr',
          actor_id: 3,
          actor_name: 'Codex',
          created_at: '2026-03-07 14:00:00',
          updated_at: '2026-03-08 08:30:00',
        },
      ],
      someday: [
        {
          id: 301,
          title: 'Sketch mobile inbox gestures',
          state: 'someday',
          sphere: 'private',
          context_ids: [3],
          artifact_id: 503,
          source: '',
          source_ref: '',
          artifact_title: 'Gesture backlog',
          artifact_kind: 'plan_note',
          actor_name: '',
          created_at: '2026-03-06 12:00:00',
          updated_at: '2026-03-06 12:00:00',
        },
      ],
      done: [
        {
          id: 401,
          title: 'Ship capture flow',
          state: 'done',
          sphere: 'private',
          context_ids: [3],
          artifact_id: 504,
          source: 'github',
          source_ref: 'owner/repo#90',
          artifact_title: 'Capture checklist',
          artifact_kind: 'github_issue',
          actor_id: 1,
          actor_name: 'Alice',
          created_at: '2026-03-05 11:00:00',
          updated_at: '2026-03-08 07:45:00',
        },
      ],
    });
    const artifactTaxonomy = Object.freeze({
      canonical_action_order: ['open_show', 'annotate_capture', 'compose', 'bundle_review', 'dispatch_execute', 'track_item', 'delegate_actor'],
      actions: Object.freeze({
        open_show: Object.freeze({ label: 'Open', prompt_label: 'Open / Show', description: 'Inspect or surface the artifact in context.' }),
        annotate_capture: Object.freeze({ label: 'Annotate', prompt_label: 'Annotate / Capture', description: 'Mark up the artifact or capture observations from it.' }),
        compose: Object.freeze({ label: 'Compose', prompt_label: 'Compose', description: 'Draft new content in response to the artifact.' }),
        bundle_review: Object.freeze({ label: 'Review', prompt_label: 'Bundle / Review', description: 'Gather notes, compare context, and review before action.' }),
        dispatch_execute: Object.freeze({ label: 'Dispatch', prompt_label: 'Dispatch / Execute', description: 'Send, apply, or execute the prepared result.' }),
        track_item: Object.freeze({ label: 'Track', prompt_label: 'Track as Item', description: 'Track follow-up work as an item.' }),
        delegate_actor: Object.freeze({ label: 'Delegate', prompt_label: 'Delegate to Actor', description: 'Hand the work to a specific actor.' }),
      }),
      kinds: Object.freeze({
        annotation: Object.freeze({ family: 'review_bundle', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'dispatch_execute', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        document: Object.freeze({ family: 'reference', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        email: Object.freeze({ family: 'message', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'dispatch_execute', 'track_item'], preferred_tool: 'text_note', mail_actions: true }),
        email_thread: Object.freeze({ family: 'message', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'bundle_review', 'dispatch_execute', 'track_item'], preferred_tool: 'text_note', mail_actions: true }),
        external_note: Object.freeze({ family: 'captured_note', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        external_task: Object.freeze({ family: 'action_card', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'compose', 'dispatch_execute', 'track_item', 'delegate_actor'], preferred_tool: 'pointer', mail_actions: false }),
        github_issue: Object.freeze({ family: 'proposal', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'bundle_review', 'dispatch_execute', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        github_pr: Object.freeze({ family: 'proposal', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'dispatch_execute', 'track_item', 'delegate_actor'], preferred_tool: 'pointer', mail_actions: false }),
        idea_note: Object.freeze({ family: 'planning_note', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'bundle_review', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        image: Object.freeze({ family: 'reference', canvas_surface: 'image_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'highlight', mail_actions: false }),
        markdown: Object.freeze({ family: 'reference', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        pdf: Object.freeze({ family: 'reference', canvas_surface: 'pdf_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'highlight', mail_actions: false }),
        plan_note: Object.freeze({ family: 'planning_note', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'compose', 'bundle_review', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        reference: Object.freeze({ family: 'reference', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'pointer', mail_actions: false }),
        transcript: Object.freeze({ family: 'transcript', canvas_surface: 'text_artifact', interaction_model: 'canonical_canvas', actions: ['open_show', 'annotate_capture', 'bundle_review', 'track_item'], preferred_tool: 'highlight', mail_actions: false }),
      }),
    });
    const defaultMeetingSummaryProposals = () => ([
      {
        index: 0,
        title: 'Draft the revised agenda',
        actor_name: 'Alice',
        evidence: 'ACTION: Alice will draft the revised agenda.',
      },
      {
        index: 1,
        title: 'Review the budget appendix',
        actor_name: '',
        evidence: 'TODO: review the budget appendix.',
      },
    ]);
