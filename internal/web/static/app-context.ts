import { LIVE_SESSION_HOTWORD_DEFAULT } from './live-session.js';
import { SLOPSHELL_CIRCLE_LAYOUT } from './slopshell-circle-contract.js';

export const refs: Record<string, any> = {};

export function setAppRefs(next) {
  Object.assign(refs, next || {});
}

export const COMPANION_VIEW_PATH_PREFIX = '__slopshell_companion__';
export const COMPANION_TRANSCRIPT_VIEW_PATH = `${COMPANION_VIEW_PATH_PREFIX}/transcript`;
export const COMPANION_SUMMARY_VIEW_PATH = `${COMPANION_VIEW_PATH_PREFIX}/summary`;
export const COMPANION_REFERENCES_VIEW_PATH = `${COMPANION_VIEW_PATH_PREFIX}/references`;
export const MEETING_TRANSCRIPT_LABEL = 'Meeting Transcript';
export const MEETING_SUMMARY_LABEL = 'Meeting Summary';
export const MEETING_REFERENCES_LABEL = 'Meeting References';
export const MEETING_SUMMARY_ITEMS_PANEL_ID = 'meeting-summary-items';
export const CHAT_CTRL_LONG_PRESS_MS = 180;
export const ARTIFACT_EDIT_LONG_TAP_MS = 420;
export const ITEM_SIDEBAR_VIEWS = ['inbox', 'next', 'waiting', 'deferred', 'someday', 'review', 'done'];
export const ITEM_SIDEBAR_GESTURE_CANCEL_PX = 12;
export const ITEM_SIDEBAR_GESTURE_COMMIT_PX = 50;
export const ITEM_SIDEBAR_GESTURE_LONG_PX = 150;
export const ITEM_SIDEBAR_DEFAULT_LATER_HOUR_UTC = 9;
export const ITEM_SIDEBAR_MENU_ID = 'item-sidebar-menu';
export const HORIZONTAL_GESTURE_MOVE_PX = 10;
export const HORIZONTAL_SWIPE_COMMIT_PX = 48;
export const HORIZONTAL_SWIPE_AXIS_RATIO = 1.25;
export const HORIZONTAL_SWIPE_HOLD_MS = 280;
export const DEV_UI_RELOAD_POLL_MS = 1500;
export const ASSISTANT_ACTIVITY_POLL_MS = 1200;
export const CHAT_WS_STALE_THRESHOLD_MS = 20000;
export const ACTIVE_TURN_NO_ID_CLEAR_GRACE_MS = 1500;
export const ACTIVE_TURN_ACTIVITY_CLEAR_GRACE_MS = 450;
export const PROJECT_CHAT_MODEL_ALIASES = ['local', 'spark', 'gpt', 'mini'];
export const PROJECT_CHAT_MODEL_REASONING_EFFORTS = {
  local: ['none'],
  spark: ['low', 'medium', 'high', 'xhigh'],
  gpt: ['low', 'medium', 'high', 'xhigh'],
  mini: ['low', 'medium', 'high', 'xhigh'],
};
export const TTS_SILENT_STORAGE_KEY = 'slopshell.ttsSilent';
export const FAST_MODE_STORAGE_KEY = 'slopshell.fastMode';
export const YOLO_MODE_STORAGE_KEY = 'slopshell.yoloMode';
export const SOMEDAY_REVIEW_NUDGE_ENABLED_STORAGE_KEY = 'slopshell.somedayReviewNudgeEnabled';
export const SOMEDAY_REVIEW_NUDGE_LAST_SHOWN_STORAGE_KEY = 'slopshell.somedayReviewNudgeLastShownAt';
export const SOMEDAY_REVIEW_NUDGE_INTERVAL_MS = 7 * 24 * 60 * 60 * 1000;
export const ACTIVE_PROJECT_STORAGE_KEY = 'slopshell.activeWorkspaceId';
export const ACTIVE_SPHERE_STORAGE_KEY = 'slopshell.activeSphere';
export const LAST_VIEW_STORAGE_KEY = 'slopshell.lastView';
export const RUNTIME_RELOAD_CONTEXT_STORAGE_KEY = 'slopshell.runtimeReloadContext';
export const TOOL_PALETTE_POSITION_STORAGE_KEY = SLOPSHELL_CIRCLE_LAYOUT.storage_key;
export const SIDEBAR_IMAGE_EXTENSIONS = new Set(['.png', '.jpg', '.jpeg', '.gif', '.webp', '.bmp', '.svg', '.ico', '.avif']);
export const PANEL_MOTION_WATCH_QUERIES = [
  '(prefers-reduced-motion: reduce)',
  '(monochrome)',
  '(update: slow)',
];
export const VOICE_LIFECYCLE = Object.freeze({
  IDLE: 'idle',
  LISTENING: 'listening',
  RECORDING: 'recording',
  STOPPING_RECORDING: 'stopping_recording',
  AWAITING_TURN: 'awaiting_turn',
  ASSISTANT_WORKING: 'assistant_working',
  TTS_PLAYING: 'tts_playing',
});
export const COMPANION_IDLE_SURFACES = Object.freeze({
  ROBOT: 'robot',
  BLACK: 'black',
});
export const COMPANION_RUNTIME_STATES = Object.freeze({
  IDLE: 'idle',
  LISTENING: 'listening',
  THINKING: 'thinking',
  TALKING: 'talking',
  ERROR: 'error',
});
export const TOOL_PALETTE_MODES = [
  {
    id: 'pointer',
    label: 'Pointer tool',
    icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 4 11 8-4.5 1.2L16 20l-2.4 1-3.5-6.9L6 17Z"/></svg>',
  },
  {
    id: 'highlight',
    label: 'Highlight tool',
    icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 15 4 4"/><path d="m8 18 8.5-8.5a2.1 2.1 0 0 0-3-3L5 15v4h4Z"/><path d="M13 7l4 4"/><path d="M4 21h16"/></svg>',
  },
  {
    id: 'ink',
    label: 'Ink tool',
    icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 20 4.5-1 9-9a2.1 2.1 0 0 0-3-3l-9 9Z"/><path d="m13 7 4 4"/><path d="M4 20h5"/></svg>',
  },
  {
    id: 'text_note',
    label: 'Text note tool',
    icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="6" width="18" height="12" rx="2"/><path d="M6 10h.01"/><path d="M9 10h.01"/><path d="M12 10h.01"/><path d="M15 10h.01"/><path d="M18 10h.01"/><path d="M6 14h12"/></svg>',
  },
  {
    id: 'prompt',
    label: 'Prompt tool',
    icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v8"/><path d="M8.5 8.5a3.5 3.5 0 0 1 7 0V12a3.5 3.5 0 0 1-7 0Z"/><path d="M6 11.5a6 6 0 0 0 12 0"/><path d="M12 17.5V21"/><path d="M9 21h6"/></svg>',
  },
];

export const SPHERE_OPTIONS = [
  { id: 'work', label: 'Work' },
  { id: 'private', label: 'Private' },
];

export const SIDEBAR_SOURCE_FILTERS = [
  { id: 'email', label: 'Email' },
  { id: 'todoist', label: 'Todoist' },
  { id: 'github', label: 'GitHub' },
  { id: 'gitlab', label: 'GitLab' },
  { id: 'markdown', label: 'Markdown' },
  { id: 'local', label: 'Local' },
];

export const state = {
  sessionId: 'local',
  canvasWs: null,
  chatWs: null,
  chatWsToken: 0,
  canvasWsToken: 0,
  chatWsHasConnected: false,
  chatSessionId: '',
  chatMode: 'chat',
  hasArtifact: false,
  projects: [],
  workspacePresets: [],
  defaultWorkspaceId: '',
  serverActiveProjectId: '',
  activeWorkspaceId: '',
  activeSphere: 'private',
  workspaceFocus: {
    anchor: null,
    focus: null,
    explicit: false,
  },
  workspaceBusyStates: [],
  projectsOpen: false,
  projectSwitchInFlight: false,
  projectModelSwitchInFlight: false,
  interaction: {
    conversation: 'push_to_talk',
    surface: 'annotate',
    tool: 'pointer',
    toolPinned: false,
  },
  startupBehavior: 'resume_active',
  ttsEnabled: false,
  ttsSilent: false,
  fastMode: false,
  yoloMode: false,
  disclaimerAckRequired: false,
  disclaimerVersion: '',
  welcomeSurface: null,
  pendingByTurn: new Map(),
  pendingApprovals: new Map(),
  pendingQueue: [],
  assistantActiveTurns: new Set(),
  assistantUnknownTurns: 0,
  assistantRemoteActiveCount: 0,
  assistantRemoteQueuedCount: 0,
  assistantLastStartedAt: 0,
  assistantCancelInFlight: false,
  assistantLastError: '',
  ttsPlaying: false,
  livePolicy: 'dialogue',
  liveSessionActive: false,
  liveSessionMode: '',
  liveSessionHotword: LIVE_SESSION_HOTWORD_DEFAULT,
  liveSessionDialogueListenActive: false,
  turnPolicyProfile: 'balanced',
  turnEvalLoggingEnabled: true,
  dialogueDiagnostics: {
    connected: false,
    sessionId: '',
    profile: 'balanced',
    evalLoggingEnabled: true,
    readyAt: 0,
    lastAction: null,
    lastMetrics: null,
    recentEvents: [],
  },
  requestedPositionPrompt: '',
  hotwordEnabled: false,
  hotwordActive: false,
  voiceTranscriptSubmitInFlight: false,
  voiceAwaitingTurn: false,
  dialogueSpeechRecognizedAt: 0,
  voiceTurns: new Set(),
  voiceLifecycle: 'idle',
  voiceLifecycleSeq: 0,
  voiceLifecycleReason: '',
  indicatorSuppressedByCanvasUpdate: false,
  chatCtrlHoldTimer: null,
  chatVoiceCapture: null,
  dictation: {
    active: false,
    targetKind: 'document_section',
    targetLabel: 'Document Section',
    prompt: '',
    artifactTitle: '',
    transcript: '',
    draftText: '',
    scratchPath: '',
    saving: false,
  },
  mailDraft: {
    artifactId: 0,
    itemId: 0,
    saveTimer: null,
    saving: false,
    sending: false,
    status: '',
  },
  mailTriage: {
    active: false,
    accountId: 0,
    accountLabel: '',
    provider: '',
    folder: '',
    filterText: '',
    queue: [],
    index: 0,
    loading: false,
    submitting: false,
    completed: 0,
    decisions: { inbox: 0, cc: 0, archive: 0, trash: 0 },
    currentMessage: null,
    prefetchedMessage: null,
    prefetchedMessageID: '',
    lastReviewId: 0,
    lastReviewAction: '',
    lastReviewMessage: null,
    undoing: false,
  },
  reasoningEffortsByAlias: {
    local: ['none'],
    spark: ['low', 'medium', 'high', 'xhigh'],
    gpt: ['low', 'medium', 'high', 'xhigh'],
    mini: ['low', 'medium', 'high', 'xhigh'],
  },
  contextUsed: 0,
  contextMax: 0,
  canvasActionThisTurn: false,
  turnFirstResponseShown: false,
  lastInputOrigin: 'text',
  pendingSubmitController: null,
  pendingSubmitKind: '',
  prReviewMode: false,
  prReviewFiles: [],
  prReviewActiveIndex: 0,
  prReviewTitle: '',
  prReviewPRNumber: '',
  prReviewDrawerOpen: false,
  fileSidebarMode: 'items',
  workspaceBrowserPath: '',
  workspaceBrowserEntries: [],
  workspaceBrowserLoading: false,
  workspaceBrowserError: '',
  workspaceBrowserActivePath: '',
  workspaceBrowserActiveIsDir: false,
  workspaceOpenFilePath: '',
  workspaceStepInFlight: false,
  currentCanvasArtifact: {
    kind: '',
    artifactKind: '',
    title: '',
    path: '',
    surfaceDefault: '',
    realArtifact: false,
    itemID: 0,
    artifactID: 0,
  },
  sidebarEdgeTapAt: 0,
  toolPalettePosition: null,
  itemSidebarView: 'inbox',
  itemSidebarFilters: {
    all_spheres: false,
    source: '',
    source_container: '',
    workspace_id: null,
    label_id: null,
    actor_id: null,
    project_item_id: null,
    workspace_unassigned: false,
    section: '',
    due_before: '',
    due_after: '',
    follow_up_before: '',
    follow_up_after: '',
  },
  itemSidebarLabelName: '',
  itemSidebarItems: [],
  itemSidebarPersonDashboard: null,
  itemSidebarCounts: { inbox: 0, next: 0, waiting: 0, deferred: 0, someday: 0, review: 0, done: 0 },
  itemSidebarSectionCounts: { project_items_open: 0, people_open: 0, drift_review: 0, dedup_review: 0, recent_meetings: 0 },
  itemSidebarSecondaryOpen: false,
  itemSidebarLoading: false,
  itemSidebarLoadSeq: 0,
  itemSidebarError: '',
  itemSidebarActiveItemID: 0,
  itemSidebarMenuOpen: false,
  itemSidebarUndoTimer: 0,
  somedayReviewNudgeEnabled: true,
  prReviewAwaitingArtifact: false,
  inkDraft: {
    strokes: [],
    activePointerId: null,
    activePointerType: '',
    activePath: null,
    target: '',
    page: 0,
    pageInner: null,
    pageWidth: 0,
    pageHeight: 0,
    draftLayer: null,
    dirty: false,
  },
  inkSubmitInFlight: false,
  companionEnabled: false,
  companionIdleSurface: 'robot',
  companionRuntimeState: 'idle',
  companionRuntimeReason: 'idle',
  companionWorkspacePath: '',
  assistantActivityTimer: null,
  assistantActivityInFlight: false,
  projectRunStatesInFlight: false,
  assistantSilentCancelInFlight: false,
  chatWsLastMessageAt: 0,
  batchStatusLabel: '',
  batchStatusActive: false,
  pendingRuntimeReloadContext: null,
  pendingRuntimeReloadStatus: '',
};

export function getState() {
  return state;
}

export function isVoiceTurn() {
  return state.lastInputOrigin === 'voice';
}
