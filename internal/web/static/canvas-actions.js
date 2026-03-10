import { refs, state } from './app-context.js';
import { artifactSupportsMailActions } from './artifact-taxonomy.js';
import {
  launchForwardAuthoring,
  launchNewMailAuthoring,
  launchReplyAllAuthoring,
  launchReplyAuthoring,
} from './app-mail-drafts.js';

function activeCanvasMailItem(event) {
  const meta = event?.meta && typeof event.meta === 'object' ? event.meta : {};
  const itemID = Number(meta?.item_id || 0);
  if (itemID <= 0) return null;
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  const item = items.find((entry) => Number(entry?.id || 0) === itemID) || null;
  const artifactKind = String(item?.artifact_kind || meta?.artifact_kind || '').trim().toLowerCase();
  if (!artifactSupportsMailActions(artifactKind)) return null;
  return item;
}

export function renderCanvasMailActions(root, event) {
  if (!(root instanceof HTMLElement)) return;
  const item = activeCanvasMailItem(event);
  if (!item) return;

  const actions = document.createElement('div');
  actions.className = 'canvas-mail-actions';

  const label = document.createElement('span');
  label.className = 'canvas-mail-actions-label';
  label.textContent = 'Mail actions';
  actions.appendChild(label);

  const newMailButton = document.createElement('button');
  newMailButton.type = 'button';
  newMailButton.className = 'edge-btn';
  newMailButton.id = 'canvas-new-mail-trigger';
  newMailButton.textContent = 'New Mail';
  newMailButton.addEventListener('click', () => {
    void launchNewMailAuthoring();
  });
  actions.appendChild(newMailButton);

  const replyButton = document.createElement('button');
  replyButton.type = 'button';
  replyButton.className = 'edge-btn';
  replyButton.id = 'canvas-reply-mail-trigger';
  replyButton.textContent = 'Reply';
  replyButton.addEventListener('click', () => {
    void launchReplyAuthoring(item);
  });
  actions.appendChild(replyButton);

  const replyAllButton = document.createElement('button');
  replyAllButton.type = 'button';
  replyAllButton.className = 'edge-btn';
  replyAllButton.id = 'canvas-reply-all-mail-trigger';
  replyAllButton.textContent = 'Reply All';
  replyAllButton.addEventListener('click', () => {
    void launchReplyAllAuthoring(item);
  });
  actions.appendChild(replyAllButton);

  const forwardButton = document.createElement('button');
  forwardButton.type = 'button';
  forwardButton.className = 'edge-btn';
  forwardButton.id = 'canvas-forward-mail-trigger';
  forwardButton.textContent = 'Forward';
  forwardButton.addEventListener('click', () => {
    void launchForwardAuthoring(item);
  });
  actions.appendChild(forwardButton);

  root.append(actions);
}

function approvalDecisionLabel(decision) {
  const value = String(decision || '').trim().toLowerCase();
  if (value === 'accept' || value === 'approve') return 'Approved';
  if (value === 'decline' || value === 'reject') return 'Rejected';
  return 'Cancelled';
}

function setCanvasApprovalButtonsDisabled(root, disabled) {
  if (!(root instanceof HTMLElement)) return;
  root.querySelectorAll('.canvas-approval-actions button[data-approval-decision]').forEach((button) => {
    if (button instanceof HTMLButtonElement) {
      button.disabled = disabled;
    }
  });
}

function showCanvasApprovalStatus(root, decision) {
  if (!(root instanceof HTMLElement)) return;
  let status = root.querySelector('.canvas-approval-status');
  if (!(status instanceof HTMLElement)) {
    status = document.createElement('div');
    status.className = 'canvas-approval-status';
    root.appendChild(status);
  }
  status.textContent = approvalDecisionLabel(decision);
}

export function renderCanvasApprovalActions(root, event) {
  if (!(root instanceof HTMLElement)) return;
  const meta = event?.meta && typeof event.meta === 'object' ? event.meta : null;
  if (!meta || meta.approval_request !== true) return;
  const requestID = String(meta.request_id || '').trim();
  if (!requestID) return;

  root.dataset.approvalRequestId = requestID;
  const panel = document.createElement('div');
  panel.className = 'canvas-approval-request';
  panel.dataset.approvalRequestId = requestID;

  const actions = document.createElement('div');
  actions.className = 'canvas-approval-actions';
  [
    ['Approve', 'accept'],
    ['Reject', 'decline'],
    ['Cancel', 'cancel'],
  ].forEach(([label, decision]) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'chat-approval-btn';
    button.dataset.approvalDecision = decision;
    button.textContent = label;
    button.addEventListener('click', () => {
      setCanvasApprovalButtonsDisabled(panel, true);
      if (typeof refs.sendChatWsJSON !== 'function' || !refs.sendChatWsJSON({ type: 'approval_response', request_id: requestID, decision })) {
        setCanvasApprovalButtonsDisabled(panel, false);
        if (typeof refs.showStatus === 'function') {
          refs.showStatus('approval send failed');
        }
      }
    });
    actions.appendChild(button);
  });
  panel.appendChild(actions);
  root.appendChild(panel);
}

export function resolveCanvasApprovalRequest(requestID, decision) {
  const key = String(requestID || '').trim();
  if (!key) return;
  const root = document.querySelector(`#canvas-text .canvas-approval-request[data-approval-request-id="${CSS.escape(key)}"]`);
  if (!(root instanceof HTMLElement)) return;
  setCanvasApprovalButtonsDisabled(root, true);
  showCanvasApprovalStatus(root, decision);
}
