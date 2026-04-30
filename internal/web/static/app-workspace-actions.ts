import * as env from './app-env.js';
import * as context from './app-context.js';

const { apiURL } = env;
const { refs, state } = context;

const showStatus = (...args) => refs.showStatus(...args);
const switchProject = (...args) => refs.switchProject(...args);
const fetchProjects = (...args) => refs.fetchProjects(...args);
const submitMessage = (...args) => refs.submitMessage(...args);
const appendPlainMessage = (...args) => refs.appendPlainMessage(...args);
const isTemporaryProjectKind = (...args) => refs.isTemporaryProjectKind(...args);

export async function createTemporaryProject(kind, sourceWorkspaceID = '') {
  const projectKind = String(kind || '').trim().toLowerCase();
  if (!isTemporaryProjectKind(projectKind)) return;
  if (state.projectSwitchInFlight || state.projectModelSwitchInFlight) return;
  showStatus(`starting ${projectKind}...`);
  const payload: Record<string, any> = {
    kind: projectKind,
    activate: true,
  };
  const sourceID = String(sourceWorkspaceID || '').trim();
  if (sourceID) {
    payload.source_workspace_id = sourceID;
  }
  try {
    const resp = await fetch(apiURL('runtime/workspaces'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const responsePayload = await resp.json();
    const project = responsePayload?.workspace || {};
    const workspaceID = String(project?.id || '').trim();
    await fetchProjects();
    if (workspaceID) {
      await switchProject(workspaceID);
      return;
    }
    showStatus(`${projectKind} ready`);
  } catch (err) {
    const message = String(err?.message || err || `${projectKind} start failed`);
    appendPlainMessage('system', `${projectKind} start failed: ${message}`);
    showStatus(`${projectKind} start failed: ${message}`);
  }
}

async function openLinkedWorkspaceAtPath(workspacePath, statusText, failurePrefix, readyText) {
  const path = String(workspacePath || '').trim();
  if (!path) return '';
  if (state.projectSwitchInFlight || state.projectModelSwitchInFlight) return '';
  showStatus(statusText);
  try {
    const resp = await fetch(apiURL('runtime/workspaces'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kind: 'linked',
        path,
        activate: true,
      }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const responsePayload = await resp.json();
    const project = responsePayload?.workspace || {};
    const workspaceID = String(project?.id || '').trim();
    await fetchProjects();
    if (workspaceID) {
      await switchProject(workspaceID);
      return workspaceID;
    }
    showStatus(readyText);
    return '';
  } catch (err) {
    const message = String(err?.message || err || 'workspace open failed');
    appendPlainMessage('system', `${failurePrefix}: ${message}`);
    showStatus(`${failurePrefix}: ${message}`);
    return '';
  }
}

export async function createLinkedWorkspaceAtPath(workspacePath) {
  await openLinkedWorkspaceAtPath(workspacePath, 'opening linked workspace...', 'Linked workspace open failed', 'linked workspace ready');
}

export async function startAgentHereAtPath(workspacePath) {
  const workspaceID = await openLinkedWorkspaceAtPath(workspacePath, 'starting agent here...', 'Start agent here failed', 'agent ready');
  if (!workspaceID) return;
  await submitMessage('Start agent here.', { kind: 'start_agent_here' });
}
